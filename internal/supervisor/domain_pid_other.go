//go:build !linux

package supervisor

import "context"

func readDomainPID(ctx context.Context, d Domain) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return 0, ErrPolicy
}
