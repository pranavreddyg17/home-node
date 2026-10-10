//go:build linux

package install

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// Caller retains an independently qualified receipt and boot identity. Admit
// only its exact directory in one of the two fixed publication locations.
func (e *Engine) withRecordedGuestStorageChannelDirectory(ctx context.Context, stage guestStorageChannelStage, legacy bool, guard func(context.Context) error, use func(*os.Root, *os.File, *os.File, func(context.Context) error) error) error {
	mode := uint32(0710)
	names := []string{".homenode-guests.stage", "guests"}
	if legacy {
		if stage.TransferGID != 0 {
			return ErrPlan
		}
		mode = 0755
		names = []string{"guests", ".homenode-guests.legacy"}
	} else if stage.TransferGID == 0 {
		return ErrPlan
	}
	return e.withGuestStorageChannelRuntime(ctx, guard, func(root *os.Root, parent *os.File, checkRuntime func(context.Context) error) (result error) {
		var runtime unix.Stat_t
		if unix.Fstat(int(parent.Fd()), &runtime) != nil || uint64(runtime.Dev) != stage.RuntimeDevice || runtime.Ino != stage.RuntimeInode {
			return ErrConflict
		}
		location := func() (string, error) {
			selected := ""
			for _, name := range names {
				var observed unix.Stat_t
				err := unix.Fstatat(int(parent.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW)
				if errors.Is(err, unix.ENOENT) {
					continue
				}
				if err != nil || selected != "" || uint64(observed.Dev) != stage.Device || observed.Ino != stage.Inode || observed.Mode != unix.S_IFDIR|mode || observed.Uid != 0 || observed.Gid != stage.TransferGID {
					return "", ErrConflict
				}
				selected = name
			}
			if selected == "" {
				return "", ErrConflict
			}
			return selected, nil
		}
		name, err := location()
		if err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		check := func(ctx context.Context) error {
			if err := checkRuntime(ctx); err != nil {
				return err
			}
			var current unix.Stat_t
			if unix.Fstat(int(file.Fd()), &current) != nil || uint64(current.Dev) != stage.Device || current.Ino != stage.Inode || current.Mode != unix.S_IFDIR|mode || current.Uid != 0 || current.Gid != stage.TransferGID {
				return ErrConflict
			}
			name, err := location()
			if err != nil {
				return err
			}
			for _, attr := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(file.Fd()), attr, nil); !errors.Is(err, unix.ENODATA) {
					return ErrConflict
				}
			}
			var childMount, parentMount unix.Statx_t
			flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
			if unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &childMount) != nil || unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &parentMount) != nil || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mnt_id != parentMount.Mnt_id {
				return ErrConflict
			}
			reader, err := root.Open(name)
			if err != nil {
				return err
			}
			observed, statErr := reader.Stat()
			retained, retainedErr := file.Stat()
			entries, readErr := reader.ReadDir(1)
			closeErr := reader.Close()
			if statErr != nil || retainedErr != nil || !os.SameFile(observed, retained) || len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
				return ErrConflict
			}
			if _, err := location(); err != nil {
				return err
			}
			return checkRuntime(ctx)
		}
		if err := check(ctx); err != nil {
			return err
		}
		if err := use(root, parent, file, check); err != nil {
			return err
		}
		return check(ctx)
	})
}
