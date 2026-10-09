//go:build !linux

package supervisor

import "context"

func ObserveReservedStorageParents(ctx context.Context, images, volumes, channels string, guestGID, accessGID uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
