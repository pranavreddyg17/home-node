//go:build !linux

package install

import "context"

func (e *Engine) RemoveGatewayAccount(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrAccounts
}
