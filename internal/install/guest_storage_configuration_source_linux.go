//go:build linux

package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
