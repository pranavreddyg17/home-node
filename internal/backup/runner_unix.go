//go:build linux || darwin

package backup

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
)

// Keep the inode after close: unlinking a lock file would let another process
// lock a replacement while the old runner still holds the original inode.
func lockMaintenanceRunner(ctx context.Context, directory string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return lockPrivateRunnerRoot(ctx, root)
}

func lockPrivateRunnerRoot(ctx context.Context, root *os.Root) (*os.File, error) {
	return lockPrivateOwnedRoot(ctx, root, uint32(os.Geteuid()), true)
}

func lockPrivateOwnedRoot(ctx context.Context, root *os.Root, expectedUID uint32, create bool) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	info, err := parent.Stat()
	var native unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 || unix.Fstat(int(parent.Fd()), &native) != nil || native.Uid != expectedUID {
		return nil, ErrMaintenanceRunner
	}
	flags := os.O_RDWR | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags |= os.O_CREATE
	}
	file, err := root.OpenFile("maintenance.lock", flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 || unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != expectedUID || native.Nlink != 1 {
		file.Close()
		return nil, ErrMaintenanceRunner
	}
	if unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		file.Close()
		return nil, ErrMaintenanceRunner
	}
	current, err := root.Lstat("maintenance.lock")
	if err != nil || !os.SameFile(info, current) {
		file.Close()
		return nil, ErrMaintenanceRunner
	}
	if err = ctx.Err(); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
