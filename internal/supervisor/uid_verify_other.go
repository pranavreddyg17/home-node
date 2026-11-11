//go:build !linux

package supervisor

import "context"

func verifyGuestDACProcess(ctx context.Context, pid int, uid, gid uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}

func verifyQEMUProcess(ctx context.Context, pid int, uid, gid uint32) error {
	return verifyGuestDACProcess(ctx, pid, uid, gid)
}

func withLivePIDFD(context.Context, int, func(func() error) error) error {
	return ErrPolicy
}
