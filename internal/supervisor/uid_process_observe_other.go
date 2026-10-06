//go:build !linux

package supervisor

import "context"

func ObserveGuestUIDProcessConflicts(ctx context.Context, pool GuestUIDPool) (GuestUIDPool, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	return GuestUIDPool{}, ErrPolicy
}
