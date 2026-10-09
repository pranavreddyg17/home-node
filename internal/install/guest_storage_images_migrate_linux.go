//go:build linux

package install

import (
	"context"
	"os"
	"reflect"
	"sort"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"golang.org/x/sys/unix"
)

// Caller holds qualified installer/runtime/account exclusion and authenticates
// this manifest against installed release trust. All image descriptors survive
// through immutable intent commitment and every ownership change. Parent and
// runtime policy publication are separate journaled transitions.
func (e *Engine) migrateGuestStorageImagesLocked(ctx context.Context, plan GuestStorageProvisioningPlan, manifest catalog.Manifest, sourceGID uint32, checkMigration func(context.Context) error) error {
	return e.migrateGuestStorageImagesWithCompletionLocked(ctx, plan, manifest, sourceGID, checkMigration, nil)
}

func (e *Engine) migrateGuestStorageImagesWithCompletionLocked(ctx context.Context, plan GuestStorageProvisioningPlan, manifest catalog.Manifest, sourceGID uint32, checkMigration func(context.Context) error, finish func(context.Context, guestStorageImagesIntent, *os.File, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(manifest.Images) != 3 || checkMigration == nil {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return err
	}
	images := append([]catalog.Image(nil), manifest.Images...)
	sort.Slice(images, func(i, j int) bool { return images[i].SHA256 < images[j].SHA256 })
	for i := 1; i < len(images); i++ {
		if images[i].SHA256 == images[i-1].SHA256 {
			return ErrPlan
		}
	}
	return e.withGuestStorageImageParent(ctx, sourceGID, checkMigration, func(root *os.Root, parent *os.File, checkParent func(context.Context) error) error {
		files := make([]*os.File, len(images))
		guards := make([]func(context.Context) error, len(images))
		checkAll := func(ctx context.Context) error {
			for _, guard := range guards {
				if guard == nil {
					return ErrPlan
				}
				if err := guard(ctx); err != nil {
					return err
				}
			}
			return checkParent(ctx)
		}
		migrate := func() error {
			if err := checkAll(ctx); err != nil {
				return err
			}
			if _, err := e.journalRoot.Lstat("guest-storage-images-intent.json"); os.IsNotExist(err) {
				receipts := make([]guestStorageImageIdentity, len(images))
				for i, image := range images {
					receipt, err := qualifyGuestStorageImage(ctx, plan, image, sourceGID, files[i])
					if err != nil {
						return err
					}
					receipts[i] = receipt
				}
				if err := checkAll(ctx); err != nil {
					return err
				}
				if err := e.commitGuestStorageImagesIntent(ctx, plan, receipts); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			return e.withGuestStorageImagesIntentGuarded(ctx, func(ctx context.Context, intent guestStorageImagesIntent, checkIntent func() error) error {
				if !reflect.DeepEqual(intent.Plan, plan) || len(intent.Images) != len(images) {
					return ErrConflict
				}
				for i, receipt := range intent.Images {
					if receipt.SHA256 != images[i].SHA256 || receipt.Bytes != images[i].Bytes || receipt.SourceGID != sourceGID {
						return ErrConflict
					}
				}
				guard := func(ctx context.Context) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					if err := checkIntent(); err != nil {
						return err
					}
					if err := checkAll(ctx); err != nil {
						return err
					}
					return checkIntent()
				}
				// Reject any mismatched image before changing the first image.
				for i, receipt := range intent.Images {
					if err := guard(ctx); err != nil {
						return err
					}
					var st unix.Stat_t
					if unix.Fstat(int(files[i].Fd()), &st) != nil || (st.Gid != receipt.SourceGID && st.Gid != receipt.GuestGID) {
						return ErrConflict
					}
					got, err := qualifyGuestStorageImage(ctx, plan, images[i], st.Gid, files[i])
					if err != nil {
						return err
					}
					got.SourceGID = receipt.SourceGID
					if got != receipt {
						return ErrConflict
					}
				}
				for i, receipt := range intent.Images {
					if err := migrateGuestStorageImage(ctx, plan, receipt, files[i], guard); err != nil {
						return err
					}
					if e.checkpoint != nil {
						if err := e.checkpoint("guest-storage-image-migrated", receipt.SHA256); err != nil {
							return err
						}
					}
				}
				for i, receipt := range intent.Images {
					if err := guard(ctx); err != nil {
						return err
					}
					got, err := qualifyGuestStorageImage(ctx, plan, images[i], receipt.GuestGID, files[i])
					if err != nil {
						return err
					}
					got.SourceGID = receipt.SourceGID
					if got != receipt {
						return ErrConflict
					}
				}
				if err := guard(ctx); err != nil {
					return err
				}
				for i, receipt := range intent.Images {
					var st unix.Stat_t
					if unix.Fstat(int(files[i].Fd()), &st) != nil || uint64(st.Dev) != receipt.Device || st.Ino != receipt.Inode || st.Mode != unix.S_IFREG|0440 || st.Uid != 0 || st.Gid != receipt.GuestGID || st.Nlink != 1 || st.Size != receipt.Bytes {
						return ErrConflict
					}
				}
				if err := e.prepareGuestStorageImageParentIntent(ctx, intent, sourceGID, parent, guard); err != nil {
					return err
				}
				if finish != nil {
					if err := guard(ctx); err != nil {
						return err
					}
					if err := finish(ctx, intent, parent, guard); err != nil {
						return err
					}
				}
				return guard(ctx)
			})
		}
		var retain func(int) error
		retain = func(index int) error {
			if index == len(images) {
				return migrate()
			}
			return withGuestStorageImagePath(ctx, root, parent, images[index].SHA256, checkParent, func(file *os.File, guard func(context.Context) error) error {
				files[index], guards[index] = file, guard
				return retain(index + 1)
			})
		}
		return retain(0)
	})
}
