//go:build linux

package install

import (
	"context"
	"encoding/json"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

type guestStorageImageParentIntent struct {
	Version            int                          `json:"version"`
	Plan               GuestStorageProvisioningPlan `json:"plan"`
	ImagesIntentSHA256 string                       `json:"imagesIntentSha256"`
	SourceGID          uint32                       `json:"sourceGid"`
	Device             uint64                       `json:"device"`
	Inode              uint64                       `json:"inode"`
}

// Called while all images, source parent, provenance and migration exclusion
// remain retained and qualified. Records source identity before a future parent
// ownership transition; this does not change the parent or release activation.
func (e *Engine) prepareGuestStorageImageParentIntent(ctx context.Context, images guestStorageImagesIntent, sourceGID uint32, parent *os.File, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if images.Version != 1 || parent == nil || guard == nil || sourceGID == 0 || sourceGID > math.MaxInt32 || os.Geteuid() != 0 {
		return ErrPlan
	}
	data, err := canonicalGuestStorageImagesIntent(ctx, images.Plan, images.Images)
	if err != nil {
		return err
	}
	for _, image := range images.Images {
		if image.SourceGID != sourceGID {
			return ErrConflict
		}
	}
	if err := guard(ctx); err != nil {
		return err
	}
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Mode != unix.S_IFDIR|0710 || original.Uid != 0 || original.Gid != sourceGID || uint64(original.Dev) > math.MaxInt64 || original.Ino == 0 || original.Ino > math.MaxInt64 {
		return ErrConflict
	}
	intent := guestStorageImageParentIntent{Version: 1, Plan: images.Plan, ImagesIntentSHA256: digest(data), SourceGID: sourceGID, Device: uint64(original.Dev), Inode: original.Ino}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	var current unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
		return ErrConflict
	}
	if err := e.commitImmutableGuestIntent(ctx, "guest-storage-image-parent-intent.json", "guest-storage-image-parent-intent", encoded); err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
		return ErrConflict
	}
	return ctx.Err()
}
