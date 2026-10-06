//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ObserveRecoveryGuestsEmpty checks one kernel cgroup-v2 snapshot of the fixed
// guest slice. It does not prevent subsequent activation or prove publication
// exclusion. Missing hierarchy is refused rather than assumed empty.
func ObserveRecoveryGuestsEmpty(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return ErrConflict
	}
	root, err := os.OpenRoot("/sys/fs/cgroup/homenode.slice")
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
	if unix.Fstatfs(int(directory.Fd()), &filesystem) != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC {
		return ErrConflict
	}
	file, err := root.OpenFile("cgroup.events", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, file}, 1025))
	if err != nil {
		return err
	}
	if err = validateRecoveryEmptyCgroup(data); err != nil {
		return err
	}
	return ctx.Err()
}
