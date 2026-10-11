//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// observeConfigurationRemovalGuestsEmpty also admits the absent slice after
// uninstall stops it. Absence is observed beneath a retained real cgroup-v2
// root, never an arbitrary host directory. Caller independently verifies owned
// manager state and holds activation exclusion; absence alone grants no lease.
func observeConfigurationRemovalGuestsEmpty(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return ErrConflict
	}
	if _, err := os.Lstat("/sys/fs/cgroup/homenode.slice"); err == nil {
		return ObserveRecoveryGuestsEmpty(ctx)
	} else if !os.IsNotExist(err) {
		return err
	}
	before, err := os.Lstat("/sys/fs/cgroup")
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return ErrConflict
	}
	root, err := os.OpenRoot("/sys/fs/cgroup")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.OpenFile(".", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return ErrConflict
	}
	var filesystem unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &filesystem) != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrConflict
	}
	if _, err := root.Lstat("homenode.slice"); !os.IsNotExist(err) {
		return ErrConflict
	}
	named, err := os.Lstat("/sys/fs/cgroup")
	if err != nil || !os.SameFile(opened, named) {
		return ErrConflict
	}
	if _, err := os.Lstat("/sys/fs/cgroup/homenode.slice"); !os.IsNotExist(err) {
		return ErrConflict
	}
	return ctx.Err()
}
