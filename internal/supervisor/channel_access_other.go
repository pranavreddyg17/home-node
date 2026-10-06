//go:build !linux

package supervisor

import "context"

func grantGuestChannelAccess(ctx context.Context, path string, uid uint32, gid int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
