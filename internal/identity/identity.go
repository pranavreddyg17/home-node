// Package identity implements the single-owner passkey and device boundary.
package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pranavreddyg17/home-node/internal/state"
)

var ErrDenied = errors.New("authorization denied or expired")
var ErrConflict = errors.New("identity state changed; start again")
var AllCapabilities = []string{"admin", "files", "jobs", "ai"}

type Service struct {
	Store *state.Store
	Web   *webauthn.WebAuthn
}
type User struct {
	ID          string
	Credentials []webauthn.Credential
}

func (u User) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u User) WebAuthnName() string                       { return "owner" }
func (u User) WebAuthnDisplayName() string                { return "HomeNode owner" }
func (u User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type Device struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	CreatedAt    int64    `json:"createdAt"`
	RevokedAt    *int64   `json:"revokedAt"`
}
type Session struct {
	Device     Device `json:"device"`
	VerifiedAt int64  `json:"verifiedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	Epoch      int64  `json:"-"`
	TokenHash  string `json:"-"`
}

func (s Session) Allows(capability string) bool {
	return slices.Contains(s.Device.Capabilities, capability)
}
func (s Session) Fresh() bool {
	now := time.Now().Unix()
	return s.VerifiedAt > 0 && s.VerifiedAt <= now && now-s.VerifiedAt < 300 && s.ExpiresAt > now
}

type challenge struct {
	Binding      *ApprovalBinding     `json:"binding,omitempty"`
	Issuer       string               `json:"issuer,omitempty"`
	Session      webauthn.SessionData `json:"session"`
	Name         string               `json:"name"`
	Capabilities []string             `json:"capabilities"`
}

func New(store *state.Store, origin string) (*Service, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	web, err := webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "HomeNode", RPOrigins: []string{origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired, ResidentKey: protocol.ResidentKeyRequirementRequired}})
	if err != nil {
		return nil, err
	}
	_, err = store.DB.Exec("INSERT OR IGNORE INTO identity(singleton,owner_id) VALUES(1,?)", state.Random()+state.Random())
	return &Service{Store: store, Web: web}, err
}

func (s *Service) Status(ctx context.Context) (claimed bool, epoch int64, err error) {
	err = s.Store.DB.QueryRowContext(ctx, "SELECT claimed,epoch FROM identity WHERE singleton=1").Scan(&claimed, &epoch)
	return
}

func ValidCapabilities(caps []string) bool {
	if len(caps) == 0 || len(caps) > len(AllCapabilities) {
		return false
	}
	seen := map[string]bool{}
	for _, cap := range caps {
		if !slices.Contains(AllCapabilities, cap) || seen[cap] {
			return false
		}
		seen[cap] = true
	}
	return true
}

// CreateSetupCode is only called by the local console command. Issuing a new
// code invalidates outstanding setup ceremonies as well as previous codes.
func (s *Service) CreateSetupCode(ctx context.Context) (string, error) {
	token := state.Random()
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var claimed bool
		var epoch int64
		if err := tx.QueryRow("SELECT claimed,epoch FROM identity WHERE singleton=1").Scan(&claimed, &epoch); err != nil {
			return err
		}
		if claimed {
			return ErrDenied
		}
		if _, err := tx.Exec("DELETE FROM invitations WHERE kind='setup'; DELETE FROM challenges WHERE kind='setup'"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO invitations(token_hash,kind,capabilities,epoch,expires_at) VALUES(?,'setup',?,?,?)", state.Hash(token), `["admin","files","jobs","ai"]`, epoch, time.Now().Add(10*time.Minute).Unix())
		return err
	})
	return token, err
}

func (s *Service) Pair(ctx context.Context, actor Session, name string, caps []string) (string, error) {
	if !actor.Allows("admin") || !actor.Fresh() || !ValidCapabilities(caps) || !ValidName(name) {
		return "", ErrDenied
	}
	token := state.Random()
	data, _ := json.Marshal(caps)
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM invitations WHERE expires_at>?", time.Now().Unix()).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return ErrDenied
		}
		if _, err := tx.Exec("INSERT INTO invitations(token_hash,issuer,kind,name,capabilities,epoch,expires_at) VALUES(?,?,'pair',?,?,?,?)", state.Hash(token), actor.Device.ID, name, string(data), actor.Epoch, time.Now().Add(5*time.Minute).Unix()); err != nil {
			return err
		}
		return state.Event(tx, actor.Device.ID, "device.invited", "", map[string]any{"capabilities": caps})
	})
	return token, err
}

func ValidName(name string) bool {
	return len(strings.TrimSpace(name)) > 0 && len(name) <= 80 && !strings.ContainsAny(name, "\x00\r\n")
}

func (s *Service) user(ctx context.Context) (User, error) {
	var u User
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT owner_id FROM identity WHERE singleton=1").Scan(&u.ID); err != nil {
		return u, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT c.data FROM credentials c JOIN devices d ON d.id=c.device_id WHERE d.revoked_at IS NULL")
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		var c webauthn.Credential
		if err = rows.Scan(&data); err != nil {
			return u, err
		}
		if err = json.Unmarshal([]byte(data), &c); err != nil {
			return u, err
		}
		u.Credentials = append(u.Credentials, c)
	}
	return u, rows.Err()
}

func (s *Service) BeginRegistration(ctx context.Context, code, name string) (any, string, error) {
	if !ValidName(name) || len(code) > 128 {
		return nil, "", ErrDenied
	}
	u, err := s.user(ctx)
	if err != nil {
		return nil, "", err
	}
	options, session, err := s.Web.BeginRegistration(u, webauthn.WithExclusions(webauthn.Credentials(u.Credentials).CredentialDescriptors()))
	if err != nil {
		return nil, "", err
	}
	token := state.Random()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var kind, caps, invitedName, issuer string
		var epoch, expires int64
		err := tx.QueryRow("DELETE FROM invitations WHERE token_hash=? RETURNING kind,name,capabilities,epoch,expires_at,coalesce(issuer,'')", state.Hash(code)).Scan(&kind, &invitedName, &caps, &epoch, &expires, &issuer)
		if err != nil {
			return ErrDenied
		}
		var claimed bool
		var current int64
		if err = tx.QueryRow("SELECT claimed,epoch FROM identity WHERE singleton=1").Scan(&claimed, &current); err != nil {
			return err
		}
		if expires <= time.Now().Unix() || epoch != current || kind == "setup" && claimed || kind == "pair" && !claimed {
			return ErrDenied
		}
		if invitedName != "" {
			name = invitedName
		}
		var capabilities []string
		if err = json.Unmarshal([]byte(caps), &capabilities); err != nil {
			return err
		}
		payload, err := json.Marshal(challenge{Session: *session, Name: name, Capabilities: capabilities, Issuer: issuer})
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,?,?,?,?)", state.Hash(token), kind, string(payload), epoch, time.Now().Add(5*time.Minute).Unix())
		return err
	})
	return options, token, err
}

func (s *Service) takeChallenge(ctx context.Context, token string) (challenge, string, int64, error) {
	var c challenge
	var kind, data string
	var epoch, expires int64
	// Consume in its own transaction: failed verification must not make a
	// challenge reusable, and a process crash must not resurrect it.
	err := s.Store.DB.QueryRowContext(ctx, "DELETE FROM challenges WHERE token_hash=? RETURNING kind,payload,epoch,expires_at", state.Hash(token)).Scan(&kind, &data, &epoch, &expires)
	if err != nil || expires <= time.Now().Unix() {
		return c, "", 0, ErrDenied
	}
	err = json.Unmarshal([]byte(data), &c)
	return c, kind, epoch, err
}

func (s *Service) FinishRegistration(ctx context.Context, token string, request *http.Request) (string, []string, error) {
	c, kind, epoch, err := s.takeChallenge(ctx, token)
	if err != nil {
		return "", nil, err
	}
	if kind != "setup" && kind != "pair" && kind != "recovery" {
		return "", nil, ErrDenied
	}
	u, err := s.user(ctx)
	if err != nil {
		return "", nil, err
	}
	credential, err := s.Web.FinishRegistration(u, c.Session, request)
	if err != nil {
		return "", nil, ErrDenied
	}
	deviceID, sessionToken := state.Random(), state.Random()
	var codes []string
	data, err := json.Marshal(credential)
	if err != nil {
		return "", nil, err
	}
	caps, _ := json.Marshal(c.Capabilities)
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var claimed bool
		var current int64
		if err := tx.QueryRow("SELECT claimed,epoch FROM identity WHERE singleton=1").Scan(&claimed, &current); err != nil {
			return err
		}
		if current != epoch || kind == "setup" && claimed || kind == "pair" && !claimed {
			return ErrConflict
		}
		if kind == "pair" {
			var active int
			if err := tx.QueryRow("SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL", c.Issuer).Scan(&active); err != nil {
				return err
			}
			if active != 1 {
				return ErrDenied
			}
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM devices WHERE revoked_at IS NULL").Scan(&count); err != nil {
			return err
		}
		if count >= 64 && kind != "recovery" {
			return ErrDenied
		}
		if kind == "recovery" {
			epoch++
			if _, err := tx.Exec("DELETE FROM sessions; DELETE FROM credentials; DELETE FROM challenges; DELETE FROM invitations; DELETE FROM recovery_codes;"); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE devices SET revoked_at=? WHERE revoked_at IS NULL", time.Now().Unix()); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE identity SET claimed=1,epoch=? WHERE singleton=1", epoch); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,?,?,?)", deviceID, c.Name, string(caps), time.Now().Unix()); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO credentials(id,device_id,data) VALUES(?,?,?)", credential.ID, deviceID, string(data)); err != nil {
			return err
		}
		if kind == "setup" || kind == "recovery" {
			for i := 0; i < 8; i++ {
				code := state.Random()
				codes = append(codes, code)
				if _, err := tx.Exec("INSERT INTO recovery_codes(token_hash) VALUES(?)", state.Hash(code)); err != nil {
					return err
				}
			}
		}
		if err := insertSession(tx, sessionToken, deviceID, epoch); err != nil {
			return err
		}
		return state.Event(tx, deviceID, "identity."+kind, deviceID, map[string]any{"epoch": epoch})
	})
	return sessionToken, codes, err
}

func (s *Service) BeginLogin(ctx context.Context) (any, string, error) {
	claimed, epoch, err := s.Status(ctx)
	if err != nil {
		return nil, "", err
	}
	if !claimed {
		return nil, "", ErrDenied
	}
	u, err := s.user(ctx)
	if err != nil {
		return nil, "", err
	}
	options, session, err := s.Web.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, "", err
	}
	token := state.Random()
	payload, err := json.Marshal(challenge{Session: *session})
	if err != nil {
		return nil, "", err
	}
	_, err = s.Store.DB.ExecContext(ctx, "INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'login',?,?,?)", state.Hash(token), string(payload), epoch, time.Now().Add(5*time.Minute).Unix())
	return options, token, err
}

func (s *Service) FinishLogin(ctx context.Context, token, oldSession string, request *http.Request) (string, error) {
	c, kind, epoch, err := s.takeChallenge(ctx, token)
	if err != nil {
		return "", err
	}
	if kind != "login" {
		return "", ErrDenied
	}
	u, err := s.user(ctx)
	if err != nil {
		return "", err
	}
	credential, err := s.Web.FinishLogin(u, c.Session, request)
	if err != nil || credential.Authenticator.CloneWarning {
		return "", ErrDenied
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return "", err
	}
	sessionToken := state.Random()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var deviceID string
		var current int64
		if err := tx.QueryRow("SELECT epoch FROM identity WHERE singleton=1").Scan(&current); err != nil {
			return err
		}
		if epoch != current {
			return ErrDenied
		}
		if err := tx.QueryRow("SELECT c.device_id FROM credentials c JOIN devices d ON d.id=c.device_id WHERE c.id=? AND d.revoked_at IS NULL", credential.ID).Scan(&deviceID); err != nil {
			return ErrDenied
		}
		if _, err := tx.Exec("UPDATE credentials SET data=? WHERE id=?", string(data), credential.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM sessions WHERE token_hash=? OR expires_at<=? OR last_seen<=?", state.Hash(oldSession), time.Now().Unix(), time.Now().Add(-30*time.Minute).Unix()); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM sessions WHERE device_id=?", deviceID).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return ErrDenied
		}
		if err := insertSession(tx, sessionToken, deviceID, epoch); err != nil {
			return err
		}
		return state.Event(tx, deviceID, "session.created", deviceID, map[string]any{})
	})
	return sessionToken, err
}

func insertSession(tx *sql.Tx, token, deviceID string, epoch int64) error {
	now := time.Now().Unix()
	_, err := tx.Exec("INSERT INTO sessions(token_hash,device_id,epoch,created_at,verified_at,last_seen,expires_at) VALUES(?,?,?,?,?,?,?)", state.Hash(token), deviceID, epoch, now, now, now, now+12*3600)
	return err
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	var session Session
	var caps string
	if token == "" || len(token) > 128 {
		return session, ErrDenied
	}
	session.TokenHash = state.Hash(token)
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		err := tx.QueryRow(`SELECT d.id,d.name,d.capabilities,d.created_at,s.verified_at,s.expires_at,s.epoch FROM sessions s JOIN devices d ON d.id=s.device_id JOIN identity i ON i.epoch=s.epoch WHERE s.token_hash=? AND d.revoked_at IS NULL AND s.expires_at>? AND s.last_seen>?`, session.TokenHash, now, now-1800).Scan(&session.Device.ID, &session.Device.Name, &caps, &session.Device.CreatedAt, &session.VerifiedAt, &session.ExpiresAt, &session.Epoch)
		if err != nil {
			return ErrDenied
		}
		if err = json.Unmarshal([]byte(caps), &session.Device.Capabilities); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE sessions SET last_seen=? WHERE token_hash=?", now, session.TokenHash)
		return err
	})
	return session, err
}

func checkActor(tx *sql.Tx, actor Session) error {
	var verified, expires, epoch int64
	var capsJSON string
	err := tx.QueryRow(`SELECT s.verified_at,s.expires_at,s.epoch,d.capabilities FROM sessions s JOIN devices d ON d.id=s.device_id JOIN identity i ON i.epoch=s.epoch WHERE s.token_hash=? AND d.id=? AND d.revoked_at IS NULL AND s.expires_at>? AND s.last_seen>?`, actor.TokenHash, actor.Device.ID, time.Now().Unix(), time.Now().Add(-30*time.Minute).Unix()).Scan(&verified, &expires, &epoch, &capsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	var caps []string
	if err := json.Unmarshal([]byte(capsJSON), &caps); err != nil {
		return err
	}
	if verified != actor.VerifiedAt || expires != actor.ExpiresAt || epoch != actor.Epoch || !slices.Equal(caps, actor.Device.Capabilities) {
		return ErrDenied
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, session Session) error {
	_, err := s.Store.DB.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", session.TokenHash)
	return err
}

func (s *Service) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,name,capabilities,created_at,revoked_at FROM devices ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Device{}
	for rows.Next() {
		var d Device
		var caps string
		if err = rows.Scan(&d.ID, &d.Name, &caps, &d.CreatedAt, &d.RevokedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(caps), &d.Capabilities); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Service) Revoke(ctx context.Context, actor Session, id string) error {
	if !actor.Allows("admin") || !actor.Fresh() {
		return ErrDenied
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		var caps string
		if err := tx.QueryRow("SELECT capabilities FROM devices WHERE id=? AND revoked_at IS NULL", id).Scan(&caps); err != nil {
			return ErrDenied
		}
		var capabilities []string
		if err := json.Unmarshal([]byte(caps), &capabilities); err != nil {
			return err
		}
		if slices.Contains(capabilities, "admin") {
			var remaining int
			if err := tx.QueryRow(`SELECT count(*) FROM devices WHERE id<>? AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')`, id).Scan(&remaining); err != nil {
				return err
			}
			if remaining == 0 {
				return ErrConflict
			}
		}
		if _, err := tx.Exec("UPDATE devices SET revoked_at=? WHERE id=?", time.Now().Unix(), id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM sessions WHERE device_id=?", id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM credentials WHERE device_id=?", id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM invitations WHERE issuer=?", id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM challenges WHERE json_extract(payload,'$.issuer')=?", id); err != nil {
			return err
		}
		return state.Event(tx, actor.Device.ID, "device.revoked", id, map[string]any{})
	})
}

// A recovery code authorizes only registering a replacement passkey. The old
// identity remains usable until that ceremony succeeds. Codes are single-use.
func (s *Service) Recovery(ctx context.Context, code string) (string, error) {
	if len(code) > 128 {
		return "", ErrDenied
	}
	token := state.Random()
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM recovery_codes WHERE token_hash=?", state.Hash(code))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrDenied
		}
		var epoch int64
		if err = tx.QueryRow("SELECT epoch FROM identity WHERE singleton=1 AND claimed=1").Scan(&epoch); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO invitations(token_hash,kind,capabilities,epoch,expires_at) VALUES(?,'recovery',?,?,?)", state.Hash(token), `["admin","files","jobs","ai"]`, epoch, time.Now().Add(5*time.Minute).Unix())
		return err
	})
	return token, err
}

func (s *Service) Cleanup(ctx context.Context) error {
	now := time.Now().Unix()
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		for _, table := range []string{"challenges", "invitations", "sessions"} {
			if _, err := tx.Exec("DELETE FROM "+table+" WHERE expires_at<=?", now); err != nil {
				return err
			}
		}
		_, err := tx.Exec("DELETE FROM events WHERE id < (SELECT coalesce(max(id),0)-10000 FROM events) AND created_at<?", now-7*86400)
		return err
	})
}
