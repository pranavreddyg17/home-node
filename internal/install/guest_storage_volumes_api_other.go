//go:build !linux

package install

import (
	"context"
	"crypto/ed25519"
)

func (e *Engine) MigrateEmptyGuestVolumeStorage(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (EmptyVolumeStorageMigrationResult, error) {
	if err := ctx.Err(); err != nil {
		return EmptyVolumeStorageMigrationResult{}, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return EmptyVolumeStorageMigrationResult{}, ErrPlan
	}
	return EmptyVolumeStorageMigrationResult{}, ErrAccounts
}
