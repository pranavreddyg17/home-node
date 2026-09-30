package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pranavreddyg17/home-node/internal/state"
	"net/http"
	"regexp"
	"slices"
	"time"
)

const approvalLifetimeSeconds int64 = 120

var approvalResource = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ApprovalBinding is the exact authority a passkey ceremony must approve.
// It is not a grant: possession of a binding never authorizes a mutation.
type ApprovalBinding struct {
	Action           string   `json:"action"`
	Resources        []string `json:"resources"`
	BodySHA256       string   `json:"bodySHA256"`
	PolicyGeneration int64    `json:"policyGeneration"`
	Epoch            int64    `json:"epoch"`
	ExpiresAt        int64    `json:"expiresAt"`
	DeviceID         string   `json:"deviceId"`
	SessionHash      string   `json:"sessionHash"`
}

// newApprovalBinding copies and canonicalizes the resource set. The digest
// includes actor/session identity so another session cannot borrow approval.
func newApprovalBinding(actor Session, action string, resources []string, body []byte, policy, now int64) (ApprovalBinding, error) {
	sum := sha256.Sum256(body)
	b := ApprovalBinding{Action: action, Resources: slices.Clone(resources), BodySHA256: hex.EncodeToString(sum[:]), PolicyGeneration: policy, Epoch: actor.Epoch, ExpiresAt: now + approvalLifetimeSeconds, DeviceID: actor.Device.ID, SessionHash: actor.TokenHash}
	slices.Sort(b.Resources)
	if !b.valid(now) {
		return ApprovalBinding{}, ErrDenied
	}
	return b, nil
}

func (b ApprovalBinding) valid(now int64) bool {
	switch b.Action {
	case "device.pair", "device.revoke", "app.start", "app.stop", "app.restart", "app.reset", "ai.delete", "backup.create", "backup.restore", "update.install":
	default:
		return false
	}
	if now <= 0 || b.PolicyGeneration <= 0 || b.Epoch <= 0 || b.ExpiresAt <= now || b.ExpiresAt-now > approvalLifetimeSeconds || !approvalResource.MatchString(b.DeviceID) || len(b.Resources) > 8 || !slices.IsSorted(b.Resources) {
		return false
	}
	for i, r := range b.Resources {
		if !approvalResource.MatchString(r) || i > 0 && r == b.Resources[i-1] {
			return false
		}
	}
	for _, digest := range []string{b.BodySHA256, b.SessionHash} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != digest {
			return false
		}
	}
	return true
}

func (b ApprovalBinding) digest() string {
	// Domain and schema prevent reuse as a session, invitation, or another hash.
	data, _ := json.Marshal(struct {
		Domain  string          `json:"domain"`
		Schema  int             `json:"schema"`
		Binding ApprovalBinding `json:"binding"`
	}{"homenode-sensitive-approval", 1, b})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// BeginApproval persists a UV-required ceremony scoped to this session's device.
// Routes must derive action/resources/body/policy themselves, never trust a
// client-supplied binding. This method alone cannot issue a grant.
func (s *Service) BeginApproval(ctx context.Context, actor Session, action string, resources []string, body []byte, policy int64) (any, string, ApprovalBinding, error) {
	now := time.Now().Unix()
	binding, err := newApprovalBinding(actor, action, resources, body, policy, now)
	if err != nil {
		return nil, "", ApprovalBinding{}, err
	}
	cap := "admin"
	if action == "ai.delete" {
		cap = "ai"
	}
	if !actor.Allows(cap) {
		return nil, "", ApprovalBinding{}, ErrDenied
	}
	var options any
	token := state.Random()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		var user User
		var claimed bool
		if err := tx.QueryRow("SELECT owner_id,claimed FROM identity WHERE singleton=1 AND epoch=?", actor.Epoch).Scan(&user.ID, &claimed); err != nil {
			return ErrDenied
		}
		if !claimed {
			return ErrDenied
		}
		rows, err := tx.Query("SELECT data FROM credentials WHERE device_id=?", actor.Device.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var data string
			var credential webauthn.Credential
			if err := rows.Scan(&data); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(data), &credential); err != nil {
				rows.Close()
				return err
			}
			user.Credentials = append(user.Credentials, credential)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(user.Credentials) == 0 {
			return ErrDenied
		}
		if _, err := tx.Exec("DELETE FROM challenges WHERE expires_at<=?", now); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM challenges WHERE kind='approval' AND json_extract(payload,'$.binding.deviceId')=?", actor.Device.ID).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return ErrDenied
		}
		assertion, ceremony, err := s.Web.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return err
		}
		ceremony.Expires = time.Unix(binding.ExpiresAt, 0)
		payload, err := json.Marshal(challenge{Session: *ceremony, Binding: &binding, Issuer: actor.Device.ID})
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval',?,?,?)", state.Hash(token), string(payload), actor.Epoch, binding.ExpiresAt); err != nil {
			return err
		}
		options = assertion
		return nil
	})
	if err != nil {
		return nil, "", ApprovalBinding{}, err
	}
	return options, token, binding, nil
}

// FinishApproval burns the ceremony even when verification fails. A verified
// assertion yields an opaque, bounded grant; only transactional consumption
// of that grant may eventually authorize its exact mutation.
func (s *Service) FinishApproval(ctx context.Context, actor Session, token string, policy int64, request *http.Request) (string, error) {
	if len(token) == 0 || len(token) > 128 {
		return "", ErrDenied
	}
	c, kind, epoch, err := s.takeChallenge(ctx, token)
	if err != nil {
		return "", err
	}
	if kind != "approval" || c.Binding == nil || !c.Binding.valid(time.Now().Unix()) || epoch != actor.Epoch || c.Binding.Epoch != actor.Epoch || c.Binding.PolicyGeneration != policy || c.Binding.DeviceID != actor.Device.ID || c.Binding.SessionHash != actor.TokenHash {
		return "", ErrDenied
	}
	cap := "admin"
	if c.Binding.Action == "ai.delete" {
		cap = "ai"
	}
	if !actor.Allows(cap) {
		return "", ErrDenied
	}
	user, err := s.user(ctx)
	if err != nil {
		return "", err
	}
	credential, err := s.Web.FinishLogin(user, c.Session, request)
	if err != nil || credential.Authenticator.CloneWarning {
		return "", ErrDenied
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return "", err
	}
	grant := state.Random()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		if !c.Binding.valid(time.Now().Unix()) {
			return ErrDenied
		}
		var device string
		if err := tx.QueryRow("SELECT c.device_id FROM credentials c JOIN devices d ON d.id=c.device_id WHERE c.id=? AND d.revoked_at IS NULL", credential.ID).Scan(&device); err != nil || device != actor.Device.ID {
			return ErrDenied
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM challenges WHERE kind='approval-grant' AND expires_at>? AND json_extract(payload,'$.binding.deviceId')=?", time.Now().Unix(), actor.Device.ID).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return ErrDenied
		}
		if _, err := tx.Exec("UPDATE credentials SET data=? WHERE id=?", string(data), credential.ID); err != nil {
			return err
		}
		payload, err := json.Marshal(challenge{Issuer: actor.Device.ID, Binding: c.Binding})
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(grant), string(payload), actor.Epoch, c.Binding.ExpiresAt)
		return err
	})
	if err != nil {
		return "", err
	}
	return grant, nil
}

// ConsumeApproval commits grant consumption and mutation together. Callers must
// derive the requested authority from the actual route/body/current policy and
// use only tx for durable mutation; external effects belong to persisted intent.
func (s *Service) ConsumeApproval(ctx context.Context, actor Session, token, action string, resources []string, body []byte, policy int64, mutation func(*sql.Tx) error) error {
	if token == "" || len(token) > 128 || mutation == nil {
		return ErrDenied
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		var payload string
		var epoch, expires int64
		if err := tx.QueryRow("SELECT payload,epoch,expires_at FROM challenges WHERE token_hash=? AND kind='approval-grant'", state.Hash(token)).Scan(&payload, &epoch, &expires); err != nil {
			return ErrDenied
		}
		var grant challenge
		if json.Unmarshal([]byte(payload), &grant) != nil || grant.Binding == nil {
			return ErrDenied
		}
		now := time.Now().Unix()
		expected, err := newApprovalBinding(actor, action, resources, body, policy, now)
		if err != nil {
			return err
		}
		expected.ExpiresAt = expires
		if !grant.Binding.valid(now) || !expected.valid(now) || epoch != actor.Epoch || grant.Binding.ExpiresAt != expires || grant.Binding.digest() != expected.digest() {
			return ErrDenied
		}
		cap := "admin"
		if action == "ai.delete" {
			cap = "ai"
		}
		if !actor.Allows(cap) {
			return ErrDenied
		}
		result, err := tx.Exec("DELETE FROM challenges WHERE token_hash=? AND kind='approval-grant'", state.Hash(token))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrDenied
		}
		return mutation(tx)
	})
}
