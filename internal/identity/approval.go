package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
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
