package supervisor

import "context"

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
		return GuestUIDPool{}, err
	}
	observed, err := ObserveLocalGuestUIDConflicts(ctx, pool)
	if err != nil {
		return GuestUIDPool{}, err
	}
	observed, err = ObserveGuestUIDProcessConflicts(ctx, observed)
	if err != nil {
		return GuestUIDPool{}, err
	}
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	return observed, nil
}
