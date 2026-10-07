//go:build linux

package supervisor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// observeGuestMemoryProcess retains the procfs process while observing its
// resource domain. It proves a bounded snapshot, not exclusion of migration.
func observeGuestMemoryProcess(ctx context.Context, pid int, id string, maximum int64) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pid <= 0 || os.Geteuid() != 0 {
		return ErrPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var filesystem unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &filesystem) != nil || filesystem.Type != unix.PROC_SUPER_MAGIC {
		return ErrPolicy
	}
	if err := qualifyGuestUIDNamespace(root); err != nil {
		return err
	}
	name := strconv.Itoa(pid)
	before, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return ErrPolicy
	}
	process, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, process.Close()) }()
	handle, err := process.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, handle.Close()) }()
	opened, err := handle.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return ErrPolicy
	}
	read := func() (data []byte, result error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := process.OpenFile("cgroup", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, ErrPolicy
		}
		data, err = io.ReadAll(io.LimitReader(file, 4097))
		if err != nil {
			return nil, err
		}
		if len(data) == 0 || len(data) > 4096 {
			return nil, ErrPolicy
		}
		return data, ctx.Err()
	}
	membership, err := read()
	if err != nil {
		return err
	}
	relative, err := guestUnifiedMembership(membership, id)
	if err != nil {
		return err
	}
	if err := observeGuestMemoryDomain(ctx, relative, id, maximum); err != nil {
		return err
	}
	current, err := read()
	if err != nil {
		return err
	}
	if !bytes.Equal(current, membership) {
		return ErrPolicy
	}
	named, err := root.Lstat(name)
	if err != nil || !os.SameFile(opened, named) {
		return ErrPolicy
	}
	return ctx.Err()
}
