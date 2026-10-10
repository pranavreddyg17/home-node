//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Retain the systemd-created runtime directory before provisioning a channel
// parent. Caller holds installer/runtime/account exclusion. This scope permits
// changes to children only; it never creates or repairs the runtime directory.
func (e *Engine) withGuestStorageChannelRuntime(ctx context.Context, guard func(context.Context) error, use func(*os.Root, *os.File, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || guard == nil || use == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	root, err := e.host.OpenRoot("run/homenode")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var initial unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &initial) != nil || initial.Mode != unix.S_IFDIR|0755 || initial.Uid != 0 || initial.Gid != 0 {
		return ErrConflict
	}
	var mount unix.Statx_t
	flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
	if unix.Statx(int(directory.Fd()), "", flags, unix.STATX_MNT_ID, &mount) != nil || mount.Mask&unix.STATX_MNT_ID == 0 || mount.Mnt_id == 0 {
		return ErrConflict
	}
	check := func(ctx context.Context) (result error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		named, err := e.host.OpenFile("run/homenode", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, named.Close()) }()
		var current, observed unix.Stat_t
		if unix.Fstat(int(directory.Fd()), &current) != nil || unix.Fstat(int(named.Fd()), &observed) != nil || current.Dev != initial.Dev || current.Ino != initial.Ino || current.Mode != initial.Mode || current.Uid != 0 || current.Gid != 0 || observed.Dev != current.Dev || observed.Ino != current.Ino || observed.Mode != current.Mode || observed.Uid != current.Uid || observed.Gid != current.Gid {
			return ErrConflict
		}
		for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
			if _, err := unix.Fgetxattr(int(directory.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
				return ErrConflict
			}
		}
		for _, file := range []*os.File{directory, named} {
			var observedMount unix.Statx_t
			if unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &observedMount) != nil || observedMount.Mask&unix.STATX_MNT_ID == 0 || observedMount.Mnt_id != mount.Mnt_id {
				return ErrConflict
			}
		}
		return guard(ctx)
	}
	if err := check(ctx); err != nil {
		return err
	}
	if err := use(root, directory, check); err != nil {
		return err
	}
	return check(ctx)
}
