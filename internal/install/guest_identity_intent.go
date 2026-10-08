package install

import (
	"context"
	"encoding/hex"
	"encoding/json"
)

type guestIdentityNameServiceIntent struct {
	Version  int                              `json:"version"`
	OwnerID  string                           `json:"ownerId"`
	Original string                           `json:"original"`
	Proposal guestIdentityNameServiceProposal `json:"proposal"`
}

// Caller holds installer exclusion and has independently observed the original
// host descriptor and owned account journal. Commitment changes no host file.
func (e *Engine) commitGuestIdentityNameServices(ctx context.Context, ownerID string, original []byte, proposal guestIdentityNameServiceProposal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := hex.DecodeString(ownerID)
	if err != nil || len(owner) != 16 || hex.EncodeToString(owner) != ownerID {
		return ErrPlan
	}
	expected, err := planGuestIdentityNameServices(original)
	if err != nil {
		return err
	}
	if proposal != expected {
		return ErrConflict
	}
	data, err := json.Marshal(guestIdentityNameServiceIntent{Version: 1, OwnerID: ownerID, Original: string(original), Proposal: expected})
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-identity-nss-intent.json", "guest-identity-nss-intent", data)
}
