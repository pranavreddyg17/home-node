//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"time"
)

// MigrateEmptyGuestVolumeStorage consumes an empty-parent proposal and independently trusted
// release key. Requires an existing activation block and dormant services/guests.
// Occupied runtime destinations are preserved and refused at this phase.
func (e *Engine) MigrateEmptyGuestVolumeStorage(ctx context.Context, publisher ed25519.PublicKey, minimum int64) (EmptyVolumeStorageMigrationResult, error) {
	empty := EmptyVolumeStorageMigrationResult{}
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
	accounts, err := e.loadAccountJournal()
	if err != nil {
		return empty, err
	}
	if !accounts.Ready || accounts.Accounts.QEMUGID <= 0 || accounts.Accounts.QEMUGID > 1<<31-1 {
		return empty, ErrConflict
	}
	sourceGID := uint32(accounts.Accounts.QEMUGID)
	if _, err := e.journalRoot.Lstat("guest-storage-volume-parent-intent.json"); os.IsNotExist(err) {
		if err := e.prepareEmptyGuestStorageVolumeParentExcludedLocked(ctx, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
			return empty, err
		}
	} else if err != nil {
		return empty, err
	}
	if err := e.publishEmptyGuestStorageVolumeParentExcludedLocked(ctx, sourceGID, publisher, minimum, observe, e.observeRecoveryDestinationVacancy); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return EmptyVolumeStorageMigrationResult{EmptyParentOwnershipMigrated: true, ParentJournalPublished: true}, nil
}
