//go:build linux

package install

import (
	"context"
	"encoding/json"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

type guestStorageVolumeParentIntent struct {
	Version   int                          `json:"version"`
	Plan      GuestStorageProvisioningPlan `json:"plan"`
	SourceGID uint32                       `json:"sourceGid"`
	Device    uint64                       `json:"device"`
	Inode     uint64                       `json:"inode"`
	Original  journal                      `json:"original"`
	Desired   journal                      `json:"desired"`
}

// Caller retains the source pathname/mount, authenticated installation and
// account/runtime exclusion. Record the exact inode and journal states before
// ownership mutation; this does not migrate populated volumes or activate guests.
func (e *Engine) commitGuestStorageVolumeParentIntent(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, sourceGID uint32, parent *os.File, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if parent == nil || guard == nil || os.Geteuid() != 0 || len(installed.Items) == 0 || len(installed.Items) > maxInstallationItems {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return err
	}
	if installed.ID != plan.Identity.OwnerID {
		return ErrConflict
	}
	desired, err := planGuestStorageVolumeParentJournal(ctx, installed, sourceGID, plan.GuestGID)
	if err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Mode != unix.S_IFDIR|0710 || original.Uid != 0 || original.Gid != sourceGID || uint64(original.Dev) > math.MaxInt64 || original.Ino == 0 || original.Ino > math.MaxInt64 {
		return ErrConflict
	}
	verify := func() error {
		if err := guard(ctx); err != nil {
			return err
		}
		var current unix.Stat_t
		if unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
			return ErrConflict
		}
		return ctx.Err()
	}
	data, err := json.Marshal(guestStorageVolumeParentIntent{Version: 1, Plan: plan, SourceGID: sourceGID, Device: uint64(original.Dev), Inode: original.Ino, Original: installed, Desired: desired})
	if err != nil {
		return err
	}
	if err := verify(); err != nil {
		return err
	}
	if err := e.commitImmutableGuestIntent(ctx, "guest-storage-volume-parent-intent.json", "guest-storage-volume-parent", data); err != nil {
		return err
	}
	return verify()
}
