//go:build !linux

package supervisor

import "context"

func ObserveGuestKVMGroup(ctx context.Context) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return 0, ErrPolicy
}
