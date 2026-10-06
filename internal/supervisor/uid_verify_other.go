//go:build !linux

package supervisor

import "context"

func verifyGuestDACProcess(ctx context.Context, pid int, uid, gid uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
