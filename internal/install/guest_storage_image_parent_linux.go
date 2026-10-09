//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Caller holds installer and migration exclusion. This retains the installed
// source parent without changing it; parent ownership publication requires its
// separate coordinated journal transition.
func (e *Engine) withGuestStorageImageParent(ctx context.Context, sourceGID uint32, checkMigration func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sourceGID == 0 || sourceGID > 1<<31-1 || checkMigration == nil || use == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	if err := checkMigration(ctx); err != nil {
		return err
	}
	const path = "var/lib/homenode/images"
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
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Mode != unix.S_IFDIR|0710 || original.Uid != 0 || original.Gid != sourceGID {
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
		if unix.Fstat(int(parent.Fd()), &current) != nil || unix.Fstat(int(named.Fd()), &namedStat) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || namedStat.Dev != current.Dev || namedStat.Ino != current.Ino || namedStat.Mode != current.Mode || namedStat.Uid != current.Uid || namedStat.Gid != current.Gid {
			return ErrConflict
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
