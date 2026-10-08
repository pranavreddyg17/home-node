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

// Retain a qualified original host file across replacement staging preparation.
// The consumer must retain allocation/runtime exclusion and journal staging
// ownership, and must leave the original namespace intact. Final replacement
// requires a separate atomic transaction with its own reconciliation checks.
func (e *Engine) withGuestIdentityHostSource(ctx context.Context, intent guestIdentityNameServiceIntent, use func(context.Context, *os.File) error) error {
	if use == nil {
		return ErrPlan
	}
	return e.withGuestIdentityNameServiceIntent(ctx, intent, func(ctx context.Context) error {
		return e.withGuestIdentityConfigurationSource(ctx, "nsswitch.conf", intent.Original, func(ctx context.Context, file *os.File, _ func() error) error { return use(ctx, file) })
	})
}

// Retained source admission for the two fixed identity configuration files.
// Callers retain the matching immutable intent and runtime/allocation guards.
func (e *Engine) withGuestIdentityConfigurationSource(ctx context.Context, name, original string, use func(context.Context, *os.File, func() error) error) (result error) {
	maximum := 8192
	if name == "login.defs" {
		maximum = maxGuestUIDAllocatorConfigurationBytes
	} else if name != "nsswitch.conf" {
		return ErrPlan
	}
	if use == nil || len(original) == 0 || len(original) > maximum {
		return ErrPlan
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, err := e.host.OpenRoot("etc")
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
	if unix.Fstat(int(file.Fd()), &initial) != nil || initial.Mode != unix.S_IFREG|0644 || initial.Nlink != 1 || initial.Uid != 0 || initial.Gid != 0 || initial.Size != int64(len(original)) {
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
		path, pathErr := e.host.Lstat("etc")
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
