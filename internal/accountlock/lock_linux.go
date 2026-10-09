//go:build linux

package accountlock

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Coordinate normal libc/shadow and systemd account writers using the shared
// whole-file record lock. OFD locking stays with this descriptor; closing an
// unrelated descriptor cannot silently release it. Direct privileged writes
// and allocators that bypass the shared protocol remain outside this barrier.
// Caller qualifies and retains the host directory and installation authority.
func With(ctx context.Context, directory *os.Root, use func(context.Context, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || use == nil || os.Geteuid() != 0 {
		return ErrInvalid
	}
	parent, err := directory.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var parentStat unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &parentStat) != nil || parentStat.Mode&unix.S_IFMT != unix.S_IFDIR || parentStat.Uid != 0 || parentStat.Gid != 0 || parentStat.Mode&0022 != 0 {
		return ErrConflict
	}
	file, err := directory.OpenFile(".pwd.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	var initial unix.Stat_t
	if unix.Fstat(int(file.Fd()), &initial) != nil || initial.Mode != unix.S_IFREG|0600 || initial.Nlink != 1 || initial.Uid != 0 || initial.Gid != 0 || initial.Size != 0 {
		return ErrConflict
	}
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart, Start: 0, Len: 0}
	if err := unix.FcntlFlock(file.Fd(), unix.F_OFD_SETLK, &lock); err != nil {
		return errors.Join(ErrConflict, err)
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current, named unix.Stat_t
		if unix.Fstat(int(file.Fd()), &current) != nil || unix.Fstatat(int(parent.Fd()), ".pwd.lock", &named, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != initial.Dev || current.Ino != initial.Ino || named.Dev != current.Dev || named.Ino != current.Ino || current.Mode != initial.Mode || current.Nlink != 1 || current.Uid != 0 || current.Gid != 0 || current.Size != 0 {
			return ErrConflict
		}
		if err := unix.FcntlFlock(file.Fd(), unix.F_OFD_SETLK, &lock); err != nil {
			return errors.Join(ErrConflict, err)
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := parent.Sync(); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if err := use(ctx, check); err != nil {
		return err
	}
	return check()
}
