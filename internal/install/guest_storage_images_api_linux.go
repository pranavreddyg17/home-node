//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"time"
)

// MigrateGuestImageStorage consumes the saved proposal and independently trusted
// release key. Requires an existing activation block and dormant services/guests.
// Occupied runtime destinations are preserved and refused at this phase.
func (e *Engine) MigrateGuestImageStorage(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (ImageStorageMigrationResult, error) {
	empty := ImageStorageMigrationResult{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return empty, ErrPlan
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
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
	if _, err := e.journalRoot.Lstat("guest-storage-image-parent-journal.json"); os.IsNotExist(err) {
		if err := e.migrateInstalledGuestStorageImagesLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
			return empty, err
		}
	} else if err != nil {
		return empty, err
	}
	if err := e.publishInstalledGuestStorageImageParentLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return ImageStorageMigrationResult{ImageOwnershipMigrated: true, ParentJournalPublished: true}, nil
}
