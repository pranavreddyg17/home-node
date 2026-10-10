//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"reflect"
	"time"
)

// Caller holds e.mu. Provision the recorded empty channel parent under live
// allocation, activation and account exclusion and independently trusted images.
// An existing receipt is consumed on retry; foreign directories are preserved.
func (e *Engine) provisionGuestStorageChannelExcludedLocked(ctx context.Context, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
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
		return e.withGuestStorageDirectoryParentState(ctx, "var/lib/homenode/volumes", plan.GuestGID, plan.GuestGID, 0, 0, false, qualified, func(_ *os.Root, _ *os.File, checkVolumes func(context.Context) error) error {
			if err := e.prepareGuestStorageChannelRuntime(ctx, checkVolumes); err != nil {
				return err
			}
			transferGID := uint32(accounts.Accounts.TransferGID)
			if _, err := e.journalRoot.Lstat("guest-storage-channel-stage.json"); err == nil {
				previous, err := e.loadGuestStorageChannelStage(ctx, plan, transferGID)
				if err != nil {
					return err
				}
				bootID, err := observeGuestStorageBootID(ctx)
				if err != nil {
					return err
				}
				if previous.BootID != bootID {
					if err := e.archivePreviousBootGuestStorageChannelStage(ctx, plan, transferGID, checkVolumes); err != nil {
						return err
					}
				}
			} else if !os.IsNotExist(err) {
				return err
			}
			if _, err := e.journalRoot.Lstat("guest-storage-channel-stage.json"); os.IsNotExist(err) {
				if _, err := e.stageGuestStorageChannelParent(ctx, plan, transferGID, checkVolumes); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			stage, err := e.loadGuestStorageChannelStage(ctx, plan, transferGID)
			if err != nil {
				return err
			}
			return e.publishGuestStorageChannelParent(ctx, plan, transferGID, stage, checkVolumes)
		})
	})
}
