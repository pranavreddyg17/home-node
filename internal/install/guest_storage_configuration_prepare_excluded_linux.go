//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"reflect"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Caller holds e.mu. Capture and stage configuration only under retained
// activation, account and live allocation exclusion. All storage parents must
// already have their final ownership. This does not publish configuration or
// release the activation block. Unrecorded partial stages remain evidence.
func (e *Engine) prepareGuestStorageConfigurationExcludedLocked(ctx context.Context, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || os.Geteuid() != 0 || e.host.Name() != "/" || len(publisher) != ed25519.PublicKeySize || minimum < 1 || observe == nil || destinations == nil {
		return ErrPlan
	}
	return e.withQualifiedGuestStorageMigrationLocked(ctx, observe, destinations, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkMigration func(context.Context) error) error {
		installed, err := e.load()
		if err != nil {
			return err
		}
		accounts, err := e.loadAccountJournal()
		if err != nil {
			return err
		}
		if !accounts.Ready || accounts.OwnerID != plan.Identity.OwnerID || accounts.Accounts.TransferGID <= 0 || accounts.Accounts.TransferGID > 1<<31-1 {
			return ErrConflict
		}
		checkStorage := func(ctx context.Context) error {
			if err := checkMigration(ctx); err != nil {
				return err
			}
			current, err := e.load()
			if err != nil || !reflect.DeepEqual(current, installed) {
				return ErrConflict
			}
			currentAccounts, err := e.loadAccountJournal()
			if err != nil || !reflect.DeepEqual(currentAccounts, accounts) {
				return ErrConflict
			}
			if err := supervisor.ObserveReservedStorageParents(ctx, "/var/lib/homenode/images", "/var/lib/homenode/volumes", "/run/homenode/guests", plan.GuestGID, uint32(accounts.Accounts.TransferGID)); err != nil {
				return err
			}
			return checkMigration(ctx)
		}
		manifest, group, err := e.guestStorageCatalogForJournal(ctx, installed, publisher, minimum, time.Now(), checkStorage)
		if err != nil {
			return err
		}
		if group != plan.GuestGID {
			return ErrConflict
		}
		qualified := func(ctx context.Context) error {
			current, gid, err := e.guestStorageCatalogForJournal(ctx, installed, publisher, minimum, time.Now(), checkStorage)
			if err != nil {
				return err
			}
			if gid != group || !reflect.DeepEqual(current, manifest) {
				return ErrConflict
			}
			for _, image := range manifest.Images {
				if err := e.verifyGuestStorageConfigurationImage(ctx, image, plan.GuestGID, checkStorage); err != nil {
					return err
				}
			}
			return checkStorage(ctx)
		}
		if err := e.prepareGuestStorageConfigurationIntent(ctx, installed, plan, qualified); err != nil {
			return err
		}
		if _, err := e.journalRoot.Lstat("guest-storage-configuration-stage.json"); os.IsNotExist(err) {
			_, err := e.stageGuestStorageConfiguration(ctx, installed, plan, qualified)
			return err
		} else if err != nil {
			return err
		}
		// A receipt authorizes only its retained original/replacement inodes;
		// matching bytes in a foreign stage never authorize adoption on retry.
		return e.withGuestStorageConfigurationStaged(ctx, installed, plan, qualified, func(_ guestStorageConfigurationStage, _ []*os.File, check func() error) error {
			return check()
		})
	})
}
