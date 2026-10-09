//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"reflect"
	"time"
)

// Caller holds e.mu and supplies exact installed destination admission.
// Completes only image ownership migration; leaves parent, policy and activation
// unchanged until their coordinated journal transitions are implemented.
func (e *Engine) migrateInstalledGuestStorageImagesLocked(ctx context.Context, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
	return e.withQualifiedGuestStorageMigrationLocked(ctx, observe, destinations, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkMigration func(context.Context) error) error {
		manifest, sourceGID, err := e.installedGuestStorageCatalog(ctx, publisher, minimum, time.Now())
		if err != nil {
			return err
		}
		guard := func(ctx context.Context) error {
			if err := checkMigration(ctx); err != nil {
				return err
			}
			current, gid, err := e.installedGuestStorageCatalog(ctx, publisher, minimum, time.Now())
			if err != nil {
				return err
			}
			if gid != sourceGID || !reflect.DeepEqual(current, manifest) {
				return ErrConflict
			}
			return checkMigration(ctx)
		}
		return e.migrateGuestStorageImagesLocked(ctx, plan, manifest, sourceGID, guard)
	})
}
