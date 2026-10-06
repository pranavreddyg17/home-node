//go:build !linux

package supervisor

import "context"

func prepareGuestChannelDirectory(ctx context.Context, path string, uid, gid int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
