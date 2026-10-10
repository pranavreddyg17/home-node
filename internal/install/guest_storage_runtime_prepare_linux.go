//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Caller holds independently qualified installed/runtime/account authority.
// Create the fixed outer directory only if absent. Existing metadata is never
// repaired; subsequent channel staging records its exact runtime inode.
func (e *Engine) prepareGuestStorageChannelRuntime(ctx context.Context, guard func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || guard == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	root, err := e.host.OpenRoot("run")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var before unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &before) != nil || before.Mode != unix.S_IFDIR|0755 || before.Uid != 0 || before.Gid != 0 {
		return ErrConflict
	}
	var mount unix.Statx_t
	flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
	if unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &mount) != nil || mount.Mask&unix.STATX_MNT_ID == 0 || mount.Mnt_id == 0 {
		return ErrConflict
	}
	check := func(ctx context.Context) (result error) {
		if err := guard(ctx); err != nil {
			return err
		}
		named, err := e.host.OpenFile("run", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, named.Close()) }()
		for _, file := range []*os.File{parent, named} {
			var observed unix.Stat_t
			var observedMount unix.Statx_t
			if unix.Fstat(int(file.Fd()), &observed) != nil || observed.Dev != before.Dev || observed.Ino != before.Ino || observed.Mode != before.Mode || observed.Uid != 0 || observed.Gid != 0 || unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &observedMount) != nil || observedMount.Mask&unix.STATX_MNT_ID == 0 || observedMount.Mnt_id != mount.Mnt_id {
				return ErrConflict
			}
		}
		for _, attr := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
			if _, err := unix.Fgetxattr(int(parent.Fd()), attr, nil); !errors.Is(err, unix.ENODATA) {
				return ErrConflict
			}
		}
		return ctx.Err()
	}
	if err := check(ctx); err != nil {
		return err
	}
	if _, err := root.Lstat("homenode"); os.IsNotExist(err) {
		if err := root.Mkdir("homenode", 0755); err != nil {
			return err
		}
		if err := parent.Sync(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return e.withGuestStorageChannelRuntime(ctx, check, func(_ *os.Root, _ *os.File, recheck func(context.Context) error) error { return recheck(ctx) })
}
