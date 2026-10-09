//go:build !linux

package supervisor

import "context"

func QualifyReservedGuestPolicy(ctx context.Context, policy ReservedGuestPolicy, serviceUIDs, serviceGIDs []uint32) (GuestUIDPool, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	return GuestUIDPool{}, ErrPolicy
}
