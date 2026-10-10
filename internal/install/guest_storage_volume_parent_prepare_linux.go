//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"os"
	"reflect"
	"time"
)

// Caller holds e.mu. Record an empty source parent only after independent live
// identity, signed catalog and retained runtime/account exclusion qualification.
// Recovery after ownership changes consumes the saved intent instead of adopting
// a new source. No successful preparation changes ownership or activation.
func (e *Engine) prepareEmptyGuestStorageVolumeParentExcludedLocked(ctx context.Context, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
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
		var sourceGID uint32
		for _, item := range installed.Items {
			if item.Path != "var/lib/homenode/volumes" {
				continue
			}
			if !item.Directory || item.UID != 0 || item.GID <= 0 || item.GID > 1<<31-1 || item.Mode != 0710 || sourceGID != 0 {
				return ErrConflict
			}
			sourceGID = uint32(item.GID)
		}
		accounts, err := e.loadAccountJournal()
		if err != nil {
			return err
		}
		if !accounts.Ready || accounts.OwnerID != plan.Identity.OwnerID || accounts.Accounts.QEMUGID <= 0 || accounts.Accounts.QEMUGID > 1<<31-1 || sourceGID != uint32(accounts.Accounts.QEMUGID) {
			return ErrConflict
		}
		if sourceGID == 0 {
			return ErrConflict
		}
		manifest, group, err := e.guestStorageCatalogForJournal(ctx, installed, publisher, minimum, time.Now(), checkMigration)
		if err != nil {
			return err
		}
		if group != plan.GuestGID {
			return ErrConflict
		}
		qualified := func(ctx context.Context) error {
			observed, err := e.load()
			if err != nil || !reflect.DeepEqual(observed, installed) {
				return ErrConflict
			}
			current, gid, err := e.guestStorageCatalogForJournal(ctx, installed, publisher, minimum, time.Now(), checkMigration)
			if err != nil {
				return err
			}
			if gid != group || !reflect.DeepEqual(current, manifest) {
				return ErrConflict
			}
			for _, image := range manifest.Images {
				if err := e.verifyGuestStorageConfigurationImage(ctx, image, plan.GuestGID, checkMigration); err != nil {
					return err
				}
			}
			return checkMigration(ctx)
		}
		return e.withGuestStorageDirectoryParentState(ctx, "var/lib/homenode/volumes", sourceGID, sourceGID, 0, 0, false, qualified, func(root *os.Root, parent *os.File, checkPath func(context.Context) error) error {
			guard := func(ctx context.Context) error {
				if err := checkPath(ctx); err != nil {
					return err
				}
				reader, err := root.Open(".")
				if err != nil {
					return err
				}
				entries, readErr := reader.ReadDir(1)
				closeErr := reader.Close()
				if len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
					return ErrConflict
				}
				return checkPath(ctx)
			}
			return e.commitGuestStorageVolumeParentIntent(ctx, installed, plan, sourceGID, parent, guard)
		})
	})
}
