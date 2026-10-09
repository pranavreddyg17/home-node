//go:build linux

package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// Retain an exact host configuration source through staging. The consumer must
// retain runtime exclusion and the matching immutable configuration intent.
// Publication requires a separate transaction and reconciliation authority.
func (e *Engine) withGuestStorageConfigurationSource(ctx context.Context, name, original string, use func(context.Context, *os.File, func() error) error) (result error) {
	maximum := 16384
	mode := uint32(0644)
	if name == "runtime-policy.json" {
		mode = 0600
	} else if name != "services.env" {
		return ErrPlan
	}
	if use == nil || len(original) == 0 || len(original) > maximum {
		return ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, err := e.host.OpenRoot("etc/homenode")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	parentFile, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parentFile.Close()) }()
	var directory unix.Stat_t
	if unix.Fstat(int(parentFile.Fd()), &directory) != nil || directory.Mode&unix.S_IFMT != unix.S_IFDIR || directory.Uid != 0 || directory.Gid != 0 || directory.Mode&0022 != 0 {
		return ErrConflict
	}
	file, err := parent.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	var initial unix.Stat_t
	if unix.Fstat(int(file.Fd()), &initial) != nil || initial.Mode != unix.S_IFREG|mode || initial.Nlink != 1 || initial.Uid != 0 || initial.Gid != 0 || initial.Size != int64(len(original)) {
		return ErrConflict
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(file, int64(len(original))+1))
		if err != nil {
			return err
		}
		var current, named unix.Stat_t
		if !bytes.Equal(data, []byte(original)) || unix.Fstat(int(file.Fd()), &current) != nil || unix.Fstatat(int(parentFile.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != initial.Dev || current.Ino != initial.Ino || current.Mode != initial.Mode || current.Nlink != 1 || current.Uid != 0 || current.Gid != 0 || current.Size != initial.Size || named.Dev != current.Dev || named.Ino != current.Ino {
			return ErrConflict
		}
		opened, err := parent.Stat(".")
		path, pathErr := e.host.Lstat("etc/homenode")
		var currentDirectory unix.Stat_t
		if unix.Fstat(int(parentFile.Fd()), &currentDirectory) != nil || currentDirectory.Dev != directory.Dev || currentDirectory.Ino != directory.Ino || currentDirectory.Mode != directory.Mode || currentDirectory.Uid != 0 || currentDirectory.Gid != 0 || err != nil || pathErr != nil || !os.SameFile(opened, path) || path.Mode().Perm()&0022 != 0 || !owned(path, 0) {
			return ErrConflict
		}
		var parentMount, pathMount, fileMount, namedMount unix.Statx_t
		flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
		if unix.Statx(int(parentFile.Fd()), "", flags, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(unix.AT_FDCWD, filepath.Join(e.host.Name(), "etc/homenode"), unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &pathMount) != nil || unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &fileMount) != nil || unix.Statx(int(parentFile.Fd()), name, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &namedMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || pathMount.Mask&unix.STATX_MNT_ID == 0 || fileMount.Mask&unix.STATX_MNT_ID == 0 || namedMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != pathMount.Mnt_id || parentMount.Mnt_id != fileMount.Mnt_id || fileMount.Mnt_id != namedMount.Mnt_id {
			return ErrConflict
		}
		for _, descriptor := range []*os.File{parentFile, file} {
			for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(descriptor.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
					return ErrConflict
				}
			}
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	if err := use(ctx, file, check); err != nil {
		return err
	}
	return check()
}
