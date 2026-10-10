//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"time"
)

// MigrateGuestStorageConfiguration publishes recorded policy and environment
// replacements while retaining the existing service activation block.
func (e *Engine) MigrateGuestStorageConfiguration(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (GuestStorageConfigurationMigrationResult, error) {
	empty := GuestStorageConfigurationMigrationResult{}
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
	if _, err := e.journalRoot.Lstat("guest-storage-configuration-stage.json"); os.IsNotExist(err) {
		if err := e.prepareGuestStorageConfigurationExcludedLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
			return empty, err
		}
	} else if err != nil {
		return empty, err
	}
	if err := e.publishGuestStorageConfigurationExcludedLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return GuestStorageConfigurationMigrationResult{PolicyPublished: true, EnvironmentPublished: true, ConfigurationJournalPublished: true}, nil
}
