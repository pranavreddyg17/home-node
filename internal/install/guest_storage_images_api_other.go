//go:build !linux

package install

import (
	"context"
	"crypto/ed25519"
)

func (e *Engine) MigrateGuestImageStorage(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (ImageStorageMigrationResult, error) {
	if err := ctx.Err(); err != nil {
		return ImageStorageMigrationResult{}, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return ImageStorageMigrationResult{}, ErrPlan
	}
	return ImageStorageMigrationResult{}, ErrAccounts
}
