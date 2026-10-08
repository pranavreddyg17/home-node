package supervisor

import (
	"context"
	"fmt"
)

// ObserveGuestUIDPoolEligibility combines protected NSS, local account/delegation
// and host task observations before first pool provisioning. It returns conflicts
// for the caller to exclude; it never reserves IDs or authorizes activation.
// Preventing future host account allocations remains the provisioner's duty.
// Do not use this first-provisioning observation to audit an already running
// pool: its own guest processes intentionally occupy their assigned UIDs.
func ObserveGuestUIDPoolEligibility(ctx context.Context, pool GuestUIDPool) (GuestUIDPool, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	if err := pool.validate(); err != nil {
		return GuestUIDPool{}, err
	}
	if err := ObserveGuestUIDNameServiceEligibility(ctx); err != nil {
		return GuestUIDPool{}, fmt.Errorf("qualify guest UID name services: %w", err)
	}
	if err := ObserveGuestUIDAutomaticAllocation(ctx, pool); err != nil {
		return GuestUIDPool{}, fmt.Errorf("qualify automatic UID allocation ranges: %w", err)
	}
	observed, err := ObserveLocalGuestUIDConflicts(ctx, pool)
	if err != nil {
		return GuestUIDPool{}, fmt.Errorf("observe local UID conflicts: %w", err)
	}
	observed, err = ObserveGuestUIDProcessConflicts(ctx, observed)
	if err != nil {
		return GuestUIDPool{}, fmt.Errorf("observe running UID conflicts: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	return observed, nil
}
