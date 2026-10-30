//go:build linux

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

func readDomainPID(ctx context.Context, d Domain) (pid int, result error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if os.Geteuid() != 0 || !guestproto.ValidID(d.ID) {
		return 0, ErrPolicy
	}
	const path = "/run/libvirt/qemu"
	parent, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var initial unix.Stat_t
	if err := unix.Fstat(int(parent.Fd()), &initial); err != nil {
		return 0, err
	}
	if initial.Mode&unix.S_IFMT != unix.S_IFDIR || initial.Uid != 0 || initial.Mode&0022 != 0 {
		return 0, fmt.Errorf("PID directory metadata uid=%d gid=%d mode=%#o: %w", initial.Uid, initial.Gid, initial.Mode, ErrPolicy)
	}
	guard := func() error {
		var current, named unix.Stat_t
		if err := unix.Fstat(int(parent.Fd()), &current); err != nil {
			return err
		}
		if err := unix.Lstat(path, &named); err != nil {
			return err
		}
		if !sameQEMUDescriptorStat(initial, current) || !sameQEMUDescriptorStat(current, named) || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, path) {
			return fmt.Errorf("PID directory binding changed: %w", ErrPolicy)
		}
		return ctx.Err()
	}
	if err := guard(); err != nil {
		return 0, err
	}
	pid, err = readDomainPIDAt(int(parent.Fd()), d)
	if err != nil {
		return 0, err
	}
	if err := guard(); err != nil {
		return 0, err
	}
	return pid, nil
}

// Metadata admission precedes opening a retained regular inode for reading.
// Caller authenticates/pins the runtime directory. No process authority follows
// from this read; the caller must still verify the live domain and credentials.
func readDomainPIDAt(parent int, d Domain) (pid int, result error) {
	if !guestproto.ValidID(d.ID) {
		return 0, ErrPolicy
	}
	name := d.Name() + ".pid"
	pathfd, err := unix.Openat(parent, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}
	defer func() { result = errors.Join(result, unix.Close(pathfd)) }()
	var before unix.Stat_t
	if err := unix.Fstat(pathfd, &before); err != nil {
		return 0, err
	}
	if before.Mode != unix.S_IFREG|0644 || before.Uid != 0 || before.Gid != 0 || before.Nlink != 1 || before.Size < 1 || before.Size > 32 {
		return 0, fmt.Errorf("PID file metadata uid=%d gid=%d mode=%#o links=%d bytes=%d: %w", before.Uid, before.Gid, before.Mode, before.Nlink, before.Size, ErrPolicy)
	}
	fd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(pathfd), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(fd), "domain-pid-value")
	defer func() { result = errors.Join(result, file.Close()) }()
	stable := func() error {
		var current, named unix.Stat_t
		if err := unix.Fstat(fd, &current); err != nil {
			return err
		}
		if err := unix.Fstatat(parent, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if !sameQEMUDescriptorStat(before, current) || !sameQEMUDescriptorStat(current, named) || before.Size != current.Size || current.Size != named.Size || before.Mtim != current.Mtim || before.Ctim != current.Ctim || current.Mtim != named.Mtim || current.Ctim != named.Ctim {
			return fmt.Errorf("PID file identity changed: %w", ErrPolicy)
		}
		return nil
	}
	if err := stable(); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil {
		return 0, err
	}
	if int64(len(data)) != before.Size {
		return 0, fmt.Errorf("PID file read size changed: %w", ErrPolicy)
	}
	pid, err = parseDomainPID(data)
	if err != nil {
		return 0, fmt.Errorf("PID file value refused: %w", err)
	}
	if err := stable(); err != nil {
		return 0, err
	}
	return pid, nil
}
