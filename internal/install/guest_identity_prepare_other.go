//go:build !linux

package install

import "context"

func (e *Engine) ApplyGuestIdentityConfiguration(ctx context.Context) (GuestIdentityConfigurationPreview, error) {
	if err := ctx.Err(); err != nil {
		return GuestIdentityConfigurationPreview{}, err
	}
	return GuestIdentityConfigurationPreview{}, ErrAccounts
}

func (e *Engine) PrepareGuestIdentityConfiguration(ctx context.Context) (GuestIdentityConfigurationPreview, error) {
	if err := ctx.Err(); err != nil {
		return GuestIdentityConfigurationPreview{}, err
	}
	return GuestIdentityConfigurationPreview{}, ErrAccounts
}
