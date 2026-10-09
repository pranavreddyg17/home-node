//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// transferReservedVolume composes durable lease authentication, namespace
// admission and the caller's retained runtime barrier with ownership transfer.
// Failures return no successful intent and preserve uncertain disk effects for
// an authenticated retry. This does not publish a runnable domain.
func (m *Manager) transferReservedVolume(ctx context.Context, directory string, d Domain, stopped func(context.Context) error) (VolumeOwnershipIntent, error) {
	var transferred VolumeOwnershipIntent
	completed := false
	err := m.withReservedVolumeOwnership(ctx, directory, d, stopped, func(ctx context.Context, file *os.File, intent VolumeOwnershipIntent, guard func(context.Context) error) error {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := transferVolumeToGuest(ctx, file, intent); err != nil {
			return err
		}
		completed = true
		if err := guard(ctx); err != nil {
			return err
		}
		transferred = intent
		return nil
	}, &completed, false)
	if err != nil {
		return VolumeOwnershipIntent{}, err
	}
	return transferred, nil
}

// withReservedVolume retains the admitted parent and disk across a consumer.
// The caller must hold runtime exclusion and supply its live stopped-guest
// guard. This scope authenticates existing intent; it never creates a lease.
func (m *Manager) withReservedVolume(ctx context.Context, directory string, d Domain, stopped func(context.Context) error, use func(context.Context, *os.File, VolumeOwnershipIntent, func(context.Context) error) error) (result error) {
	return m.withReservedVolumeOwnership(ctx, directory, d, stopped, use, nil, false)
}

func (m *Manager) withReservedVolumeOwnership(ctx context.Context, directory string, d Domain, stopped func(context.Context) error, use func(context.Context, *os.File, VolumeOwnershipIntent, func(context.Context) error) error, guestOwned *bool, publish bool) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || stopped == nil || use == nil {
		return ErrPolicy
	}
	if err := stopped(ctx); err != nil {
		return err
	}
	staged := publish
	file, err := openReservedVolumeEntry(ctx, directory, d, staged)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	root, err := os.OpenRoot(directory)
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
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Uid != 0 || original.Gid != d.GuestGID || original.Mode&unix.S_IFMT != unix.S_IFDIR || original.Mode&07777 != 0710 {
		return ErrPolicy
	}
	var originalMount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &originalMount) != nil || originalMount.Mask&unix.STATX_MNT_ID == 0 || originalMount.Mnt_id == 0 {
		return ErrPolicy
	}
	intent, err := m.verifyPinnedVolumeOwnership(ctx, d, file)
	if err != nil {
		return err
	}
	checkObjects := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current unix.Stat_t
		pinnedParent, statErr := parent.Stat()
		namedParent, pathErr := os.Lstat(directory)
		if statErr != nil || pathErr != nil || !os.SameFile(pinnedParent, namedParent) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
			return ErrPolicy
		}
		other := "." + d.ID + ".volume-prepare"
		if staged {
			other = d.ID + ".raw"
		}
		if _, err := root.Lstat(other); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrPolicy, err)
		}
		var parentMount, pathMount, diskMount unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(unix.AT_FDCWD, directory, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &pathMount) != nil || unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &diskMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || pathMount.Mask&unix.STATX_MNT_ID == 0 || diskMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id != originalMount.Mnt_id || pathMount.Mnt_id != originalMount.Mnt_id || diskMount.Mnt_id != originalMount.Mnt_id {
			return ErrPolicy
		}
		// Reopening performs complete pathname, mount-ID, allocation, mode and
		// single-link admission. It must still name the retained consumer inode.
		probe, err := openReservedVolumeEntry(ctx, directory, d, staged)
		if err != nil {
			return err
		}
		pinned, pinnedErr := file.Stat()
		named, namedErr := probe.Stat()
		closeErr := probe.Close()
		if pinnedErr != nil || namedErr != nil || !os.SameFile(pinned, named) {
			return errors.Join(ErrPolicy, pinnedErr, namedErr, closeErr)
		}
		if closeErr != nil {
			return closeErr
		}
		verified, err := m.verifyPinnedVolumeOwnership(ctx, d, file)
		if err != nil {
			return err
		}
		if verified != intent {
			return ErrPolicy
		}
		if guestOwned != nil && *guestOwned {
			var owner unix.Stat_t
			if unix.Fstat(int(file.Fd()), &owner) != nil || owner.Uid != intent.UID || owner.Gid != intent.GID {
				return ErrPolicy
			}
		}
		return nil
	}
	guard := func(ctx context.Context) error {
		if err := checkObjects(ctx); err != nil {
			return err
		}
		if err := stopped(ctx); err != nil {
			return err
		}
		return checkObjects(ctx)
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(ctx, file, intent, guard); err != nil {
		return err
	}
	if publish {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := unix.Renameat2(int(parent.Fd()), "."+d.ID+".volume-prepare", int(parent.Fd()), d.ID+".raw", unix.RENAME_NOREPLACE); err != nil {
			return err
		}
		staged = false
	}
	if err := parent.Sync(); err != nil {
		return err
	}
	return guard(ctx)
}
