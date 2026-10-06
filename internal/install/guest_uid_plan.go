package install

import (
	"context"
	"encoding/hex"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// GuestUIDProvisioningPlan is a read-only first-install proposal. Pending gates
// must be implemented and verified before publishing policy or enabling guests.
type GuestUIDProvisioningPlan struct {
	OwnerID string   `json:"ownerId"`
	First   uint32   `json:"firstUid"`
	Last    uint32   `json:"lastUid"`
	Pending []string `json:"pending"`
}

// PlanGuestUIDProvisioning observes the host; it does not modify NSS, login.defs,
// subordinate ranges, durable leases, files, devices, or runtime admission.
func PlanGuestUIDProvisioning(ctx context.Context, ownerID string, candidate supervisor.GuestUIDPool, serviceUIDs []uint32) (GuestUIDProvisioningPlan, error) {
	if _, err := guestUIDProvisioningPlan(ctx, ownerID, candidate, serviceUIDs); err != nil {
		return GuestUIDProvisioningPlan{}, err
	}
	observed, err := supervisor.ObserveGuestUIDPoolEligibility(ctx, candidate)
	if err != nil {
		return GuestUIDProvisioningPlan{}, err
	}
	return guestUIDProvisioningPlan(ctx, ownerID, observed, serviceUIDs)
}

func guestUIDProvisioningPlan(ctx context.Context, ownerID string, pool supervisor.GuestUIDPool, serviceUIDs []uint32) (GuestUIDProvisioningPlan, error) {
	empty := GuestUIDProvisioningPlan{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if len(ownerID) != 32 {
		return empty, ErrPlan
	}
	if _, err := hex.DecodeString(ownerID); err != nil {
		return empty, ErrPlan
	}
	if pool.First < 65536 || pool.Last < pool.First || pool.Last > 1<<31-1 || uint64(pool.Last)-uint64(pool.First) >= 65536 {
		return empty, ErrPlan
	}
	// The entire proposed range must be vacant; never silently shrink a policy.
	for uid := pool.First; uid <= pool.Last; uid++ {
		if pool.Blocked[uid] {
			return empty, ErrConflict
		}
	}
	seen := map[uint32]bool{}
	for _, uid := range serviceUIDs {
		if uid == 0 || uid > 1<<31-1 || seen[uid] || uid >= pool.First && uid <= pool.Last {
			return empty, ErrAccounts
		}
		seen[uid] = true
	}
	if len(serviceUIDs) < 2 {
		return empty, ErrAccounts
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return GuestUIDProvisioningPlan{OwnerID: ownerID, First: pool.First, Last: pool.Last, Pending: []string{
		"exclude future local and subordinate account allocations from the entire range",
		"journal immutable host pool policy and ownership intent",
		"verify stopped-runtime barriers and qualified volume/channel ownership",
		"qualify device and immutable image access under installed libvirt service protections",
		"verify native launch, restart, and cross-guest isolation before activation",
	}}, nil
}
