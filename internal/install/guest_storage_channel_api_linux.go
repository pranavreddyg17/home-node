//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"time"
)

// ProvisionGuestStorageChannels publishes an empty, recorded channel parent
// under the existing activation block. It never starts services or guests.
func (e *Engine) ProvisionGuestStorageChannels(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (GuestStorageChannelProvisioningResult, error) {
	empty := GuestStorageChannelProvisioningResult{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return empty, ErrPlan
	}
	if e == nil || e.host == nil || os.Geteuid() != 0 || e.host.Name() != "/" {
		return empty, ErrAccounts
	}
	if !e.mu.TryLock() {
		return empty, ErrConflict
	}
	defer e.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := e.requireRecoveryActivationBlock(ctx); err != nil {
		return empty, err
	}
	observe := func(ctx context.Context) error {
		if err := ObserveRecoveryActivationConditions(ctx); err != nil {
			return err
		}
		if err := ObserveRecoveryServicesDormant(ctx); err != nil {
			return err
		}
		return ObserveRecoveryGuestsEmpty(ctx)
	}
	if err := e.provisionGuestStorageChannelExcludedLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return GuestStorageChannelProvisioningResult{ChannelParentPublished: true}, nil
}
