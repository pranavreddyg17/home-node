//go:build !linux

package install

import (
	"context"
	"crypto/ed25519"
)

func (e *Engine) ProvisionGuestStorageChannels(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (GuestStorageChannelProvisioningResult, error) {
	if err := ctx.Err(); err != nil {
		return GuestStorageChannelProvisioningResult{}, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return GuestStorageChannelProvisioningResult{}, ErrPlan
	}
	return GuestStorageChannelProvisioningResult{}, ErrAccounts
}
