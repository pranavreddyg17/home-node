//go:build linux

package install

import (
	"context"
	"crypto/ed25519"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"golang.org/x/sys/unix"
)

// Caller holds e.mu and supplies native runtime/destination observers. This
// consumes completed image migration provenance and publishes only its parent;
// it never publishes runtime pool policy or releases activation.
func (e *Engine) publishInstalledGuestStorageImageParentLocked(ctx context.Context, publisher ed25519.PublicKey, minimum int64, observe, destinations func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 || observe == nil || destinations == nil || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrPlan
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	return e.withGuestStorageParentAuthorityLocked(ctx, func(ctx context.Context, a guestStorageParentAuthority, root *os.Root, parent *os.File, checkPath func(context.Context) error) error {
		admit := func(ctx context.Context, current journal) error {
			if err := checkPath(ctx); err != nil {
				return err
			}
			if err := e.admitGuestStorageParentInstallation(ctx, current, a.Parent, a.Transition, parent); err != nil {
				return err
			}
			return checkPath(ctx)
		}
		return e.withRecoveryInstallationExclusionGuardedLocked(ctx, observe, destinations, admit, func(ctx context.Context, checkRuntime func(context.Context) error) error {
			return e.withGuestStorageAccountExclusionLocked(ctx, checkRuntime, func(ctx context.Context, checkAccount func(context.Context) error) error {
				baseGuard := func(ctx context.Context) error {
					if err := checkPath(ctx); err != nil {
						return err
					}
					if err := checkAccount(ctx); err != nil {
						return err
					}
					live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: a.Plan.Identity.First, Last: a.Plan.Identity.Last})
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(live, a.Plan) {
						return ErrConflict
					}
					if err := checkAccount(ctx); err != nil {
						return err
					}
					return checkPath(ctx)
				}
				manifest, gid, err := e.guestStorageCatalogForJournal(ctx, a.Transition.Original, publisher, minimum, time.Now(), baseGuard)
				if err != nil {
					return err
				}
				if gid != a.Parent.SourceGID || len(manifest.Images) != len(a.Images.Images) {
					return ErrConflict
				}
				images := append([]catalog.Image(nil), manifest.Images...)
				sort.Slice(images, func(i, j int) bool { return images[i].SHA256 < images[j].SHA256 })
				for i, receipt := range a.Images.Images {
					if receipt.SHA256 != images[i].SHA256 || receipt.Bytes != images[i].Bytes {
						return ErrConflict
					}
				}
				files := make([]*os.File, len(images))
				paths := make([]func(context.Context) error, len(images))
				guard := func(ctx context.Context) error {
					if err := baseGuard(ctx); err != nil {
						return err
					}
					current, group, err := e.guestStorageCatalogForJournal(ctx, a.Transition.Original, publisher, minimum, time.Now(), baseGuard)
					if err != nil {
						return err
					}
					if group != gid || !reflect.DeepEqual(current, manifest) {
						return ErrConflict
					}
					for i, receipt := range a.Images.Images {
						if paths[i] == nil || files[i] == nil {
							return ErrPlan
						}
						if err := paths[i](ctx); err != nil {
							return err
						}
						var st unix.Stat_t
						if unix.Fstat(int(files[i].Fd()), &st) != nil || uint64(st.Dev) != receipt.Device || st.Ino != receipt.Inode || st.Mode != unix.S_IFREG|0440 || st.Uid != 0 || st.Gid != receipt.GuestGID || st.Nlink != 1 || st.Size != receipt.Bytes {
							return ErrConflict
						}
					}
					return checkPath(ctx)
				}
				verifyImages := func() error {
					if err := guard(ctx); err != nil {
						return err
					}
					for i, receipt := range a.Images.Images {
						got, err := qualifyGuestStorageImage(ctx, a.Plan, images[i], a.Plan.GuestGID, files[i])
						if err != nil {
							return err
						}
						got.SourceGID = receipt.SourceGID
						if got != receipt {
							return ErrConflict
						}
					}
					return guard(ctx)
				}
				var retain func(int) error
				retain = func(i int) error {
					if i == len(images) {
						if err := verifyImages(); err != nil {
							return err
						}
						if err := e.publishGuestStorageImageParentLocked(ctx, a.Parent, a.Transition, parent, guard); err != nil {
							return err
						}
						return verifyImages()
					}
					return withGuestStorageImagePath(ctx, root, parent, images[i].SHA256, checkPath, func(file *os.File, check func(context.Context) error) error {
						files[i], paths[i] = file, check
						return retain(i + 1)
					})
				}
				return retain(0)
			})
		})
	})
}
