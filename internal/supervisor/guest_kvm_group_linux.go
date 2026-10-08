//go:build linux

package supervisor

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

// ObserveGuestKVMGroup observes the current device DAC group without opening
// the KVM driver or modifying permissions. It is not a durable allocation or
// device-access guarantee; installation and runtime must requalify separately.
func ObserveGuestKVMGroup(ctx context.Context) (group uint32, result error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if os.Geteuid() != 0 {
		return 0, ErrPolicy
	}
	proc, err := os.OpenRoot("/proc")
	if err != nil {
		return 0, err
	}
	defer func() {
		result = errors.Join(result, proc.Close())
		if result != nil {
			group = 0
		}
	}()
	if err := qualifyGuestUIDNamespace(proc); err != nil {
		return 0, err
	}
	parent, err := openAbsoluteDirectoryNoLinks("/dev")
	if err != nil {
		return 0, err
	}
	defer func() {
		result = errors.Join(result, unix.Close(parent))
		if result != nil {
			group = 0
		}
	}()
	fd, err := unix.Openat(parent, "kvm", unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return 0, err
	}
	defer func() {
		result = errors.Join(result, unix.Close(fd))
		if result != nil {
			group = 0
		}
	}()
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFCHR || before.Mode&07777 != 0660 || before.Uid != 0 || before.Gid == 0 || before.Gid > 1<<31-1 || before.Nlink != 1 || unix.Major(uint64(before.Rdev)) != 10 || unix.Minor(uint64(before.Rdev)) != 232 {
		return 0, ErrPolicy
	}
	if !samePathMount(fd, parent, "kvm") {
		return 0, ErrPolicy
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := unix.Lgetxattr("/dev/kvm", "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
			return 0, ErrPolicy
		}
		var current unix.Stat_t
		if unix.Lstat("/dev/kvm", &current) != nil || current.Dev != before.Dev || current.Ino != before.Ino || current.Mode != before.Mode || current.Uid != before.Uid || current.Gid != before.Gid || current.Nlink != before.Nlink || current.Rdev != before.Rdev || !samePathMount(fd, parent, "kvm") || !samePathMount(fd, unix.AT_FDCWD, "/dev/kvm") {
			return 0, ErrPolicy
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return before.Gid, nil
}
