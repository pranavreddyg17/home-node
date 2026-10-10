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

// Caller holds e.mu. Publish only the already recorded empty volume parent
// while retaining migration provenance, activation block and account exclusion.
// No source inode is adopted and successful return never activates services.
func (e *Engine) publishEmptyGuestStorageVolumeParentExcludedLocked(ctx context.Context, sourceGID uint32, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || os.Geteuid() != 0 || e.host.Name() != "/" || sourceGID == 0 || sourceGID > 1<<31-1 || len(publisher) != ed25519.PublicKeySize || minimum < 1 || observe == nil || destinations == nil {
		return ErrPlan
	}
	_, err := e.withGuestStorageIntentGuarded(ctx, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkPlan func() error) error {
		current, err := e.load()
		if err != nil {
			return err
		}
		return e.withGuestStorageVolumeParentIntent(ctx, current, plan, sourceGID, func(intent guestStorageVolumeParentIntent, checkIntent func() error) error {
			provenance := func(ctx context.Context) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := checkPlan(); err != nil {
					return err
				}
				return checkIntent()
			}
			return e.withRecordedGuestStorageVolumeParent(ctx, intent, provenance, func(root *os.Root, parent *os.File, checkPath func(context.Context) error) error {
				admit := func(ctx context.Context, current journal) error {
					if err := checkPath(ctx); err != nil {
						return err
					}
					if err := e.admitEmptyGuestStorageVolumeParentInstallation(ctx, current, intent, root, parent); err != nil {
						return err
					}
					return checkPath(ctx)
				}
				return e.withRecoveryInstallationExclusionGuardedLocked(ctx, observe, destinations, admit, func(ctx context.Context, checkRuntime func(context.Context) error) error {
					return e.withGuestStorageAccountExclusionLocked(ctx, checkRuntime, func(ctx context.Context, checkAccounts func(context.Context) error) error {
						guard := func(ctx context.Context) error {
							if err := checkAccounts(ctx); err != nil {
								return err
							}
							live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: plan.Identity.First, Last: plan.Identity.Last})
							if err != nil {
								return err
							}
							if !reflect.DeepEqual(live, plan) {
								return ErrConflict
							}
							return checkPath(ctx)
						}
						manifest, group, err := e.guestStorageCatalogForJournal(ctx, intent.Original, publisher, minimum, time.Now(), guard)
						if err != nil {
							return err
						}
						if group != plan.GuestGID {
							return ErrConflict
						}
						qualified := func(ctx context.Context) error {
							current, gid, err := e.guestStorageCatalogForJournal(ctx, intent.Original, publisher, minimum, time.Now(), guard)
							if err != nil {
								return err
							}
							if gid != group || !reflect.DeepEqual(current, manifest) {
								return ErrConflict
							}
							for _, image := range manifest.Images {
								if err := e.verifyGuestStorageConfigurationImage(ctx, image, plan.GuestGID, guard); err != nil {
									return err
								}
							}
							return guard(ctx)
						}
						return e.publishEmptyGuestStorageVolumeParentLocked(ctx, plan, sourceGID, qualified)
					})
				})
			})
		})
	})
	return err
}
