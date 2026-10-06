//go:build !linux

package install

import "context"

func ObserveRecoveryServicesDormant(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrConflict
}
