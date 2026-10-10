//go:build linux

package install

import (
	"context"
	"errors"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

// Caller holds installer and migration exclusion. This retains the installed
// source parent without changing it; parent ownership publication requires its
// separate coordinated journal transition.
func (e *Engine) withGuestStorageImageParent(ctx context.Context, sourceGID uint32, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) (result error) {
	return e.withGuestStorageImageParentState(ctx, sourceGID, sourceGID, 0, 0, false, checkMigration, use)
}

// Retained parent provenance authorizes only the exact recorded inode and its
// source/destination groups. Installation-state admission remains separate.
func (e *Engine) withRecordedGuestStorageImageParent(ctx context.Context, intent guestStorageImageParentIntent, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if intent.Version != 1 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, intent.Plan); err != nil {
		return err
	}
	return e.withGuestStorageImageParentState(ctx, intent.SourceGID, intent.Plan.GuestGID, intent.Device, intent.Inode, true, checkMigration, use)
}

func (e *Engine) withGuestStorageImageParentState(ctx context.Context, sourceGID, guestGID uint32, device, inode uint64, recorded bool, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) error {
	return e.withGuestStorageDirectoryParentState(ctx, "var/lib/homenode/images", sourceGID, guestGID, device, inode, recorded, checkMigration, use)
}

func (e *Engine) withRecordedGuestStorageVolumeParent(ctx context.Context, intent guestStorageVolumeParentIntent, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if intent.Version != 1 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, intent.Plan); err != nil {
		return err
	}
	return e.withGuestStorageDirectoryParentState(ctx, "var/lib/homenode/volumes", intent.SourceGID, intent.Plan.GuestGID, intent.Device, intent.Inode, true, checkMigration, use)
}

func (e *Engine) withGuestStorageDirectoryParentState(ctx context.Context, path string, sourceGID, guestGID uint32, device, inode uint64, recorded bool, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sourceGID == 0 || sourceGID > 1<<31-1 || guestGID == 0 || guestGID > 1<<31-1 || checkMigration == nil || use == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	if err := checkMigration(ctx); err != nil {
		return err
	}
	if path != "var/lib/homenode/images" && path != "var/lib/homenode/volumes" {
		return ErrPlan
	}
	root, err := e.host.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Mode != unix.S_IFDIR|0710 || original.Uid != 0 || (original.Gid != sourceGID && original.Gid != guestGID) || (recorded && (uint64(original.Dev) != device || original.Ino != inode)) {
		return ErrConflict
	}
	var mount unix.Statx_t
	flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
	if unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &mount) != nil || mount.Mask&unix.STATX_MNT_ID == 0 || mount.Mnt_id == 0 {
		return ErrConflict
	}
	guard := func(ctx context.Context) (result error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkMigration(ctx); err != nil {
			return err
		}
		named, err := e.host.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, named.Close()) }()
		var current, namedStat unix.Stat_t
		if unix.Fstat(int(parent.Fd()), &current) != nil || unix.Fstat(int(named.Fd()), &namedStat) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || (current.Gid != sourceGID && current.Gid != guestGID) || namedStat.Dev != current.Dev || namedStat.Ino != current.Ino || namedStat.Mode != current.Mode || namedStat.Uid != current.Uid || namedStat.Gid != current.Gid {
			return ErrConflict
		}
		for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
			if _, err := unix.Fgetxattr(int(parent.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
				return ErrConflict
			}
		}
		var currentMount, namedMount unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &currentMount) != nil || unix.Statx(int(named.Fd()), "", flags, unix.STATX_MNT_ID, &namedMount) != nil || currentMount.Mask&unix.STATX_MNT_ID == 0 || namedMount.Mask&unix.STATX_MNT_ID == 0 || currentMount.Mnt_id != mount.Mnt_id || namedMount.Mnt_id != mount.Mnt_id {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(root, parent, guard); err != nil {
		return err
	}
	return guard(ctx)
}
