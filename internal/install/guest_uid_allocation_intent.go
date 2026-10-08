package install

import (
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type guestUIDAllocationIntent struct {
	Version   int                        `json:"version"`
	OwnerID   string                     `json:"ownerId"`
	First     uint32                     `json:"firstUid"`
	Last      uint32                     `json:"lastUid"`
	Selection guestUIDAllocatorRanges    `json:"selection"`
	Original  string                     `json:"original"`
	Proposal  guestUIDAllocationProposal `json:"proposal"`
}

func encodeGuestUIDAllocationIntent(ctx context.Context, intent guestUIDAllocationIntent) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, err := hex.DecodeString(intent.OwnerID)
	if intent.Version != 1 || err != nil || len(owner) != 16 || hex.EncodeToString(owner) != intent.OwnerID {
		return nil, ErrPlan
	}
	proposal, err := planGuestUIDAllocatorConfiguration(ctx, []byte(intent.Original), supervisor.GuestUIDPool{First: intent.First, Last: intent.Last}, intent.Selection)
	if err != nil {
		return nil, err
	}
	if proposal != intent.Proposal {
		return nil, ErrConflict
	}
	return json.Marshal(intent)
}

// Caller independently verifies the owner and original host descriptor. A
// committed record grants no host mutation, pool reservation or activation.
func (e *Engine) commitGuestUIDAllocationIntent(ctx context.Context, intent guestUIDAllocationIntent) error {
	data, err := encodeGuestUIDAllocationIntent(ctx, intent)
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-uid-allocation-intent.json", "guest-uid-allocation-intent", data)
}

func (e *Engine) withGuestUIDAllocationIntent(ctx context.Context, intent guestUIDAllocationIntent, use func(context.Context, func() error) error) error {
	data, err := encodeGuestUIDAllocationIntent(ctx, intent)
	if err != nil {
		return err
	}
	return e.withGuestIdentityRecordGuarded(ctx, "guest-uid-allocation-intent.json", data, use)
}
