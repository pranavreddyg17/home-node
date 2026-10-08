//go:build !linux

package install

import "context"

func (e *Engine) QuiesceRecovery(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrConflict
}

func ObserveRecoveryServicesDormant(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrConflict
}
