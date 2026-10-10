//go:build !linux

package install

import "context"

func ObserveRecoveryActivationConditions(ctx context.Context, gateway ...bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrConflict
}
