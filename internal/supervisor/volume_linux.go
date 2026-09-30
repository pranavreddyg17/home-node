//go:build linux

package supervisor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/unix"
	"math"
	"os"
	"path/filepath"
)

// New volumes are formatted before exclusive publication. Existing data is never formatted.
func prepareDataVolume(ctx context.Context, path string, size int64, reserve int64) error {
	if os.Geteuid() != 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || reserve < 4<<30 || reserve > math.MaxInt64-size || size < 16<<20 || size > 512<<30 || ctx.Err() != nil {
		return ErrPolicy
	}
	parentPath := filepath.Dir(path)
	before, err := os.Lstat(parentPath)
	if err != nil || !before.IsDir() || before.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	opened, err := parent.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return ErrPolicy
	}
	var directory unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &directory) != nil || directory.Uid != 0 {
		return ErrPolicy
	}
	name := filepath.Base(path)
	existing, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err == nil {
		defer existing.Close()
		return admitVolume(existing, size)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var space unix.Statfs_t
	if unix.Fstatfs(int(parent.Fd()), &space) != nil || space.Bsize <= 0 {
		return ErrCapacity
	}
	blocks := (uint64(size+reserve) + uint64(space.Bsize) - 1) / uint64(space.Bsize)
	if space.Bavail < blocks {
		return ErrCapacity
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	stage := name + ".prepare-" + hex.EncodeToString(nonce)
	file, err := root.OpenFile(stage, os.O_RDWR|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	// Failed staging is retained; it has never been attached to a guest.
	if err := file.Truncate(size); err != nil {
		return err
	}
	if err := admitVolume(file, size); err != nil {
		return err
	}
	if err := unix.Fallocate(int(file.Fd()), 0, 0, size); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if _, err := commandWithFiles(ctx, "", []*os.File{file}, "/usr/sbin/mkfs.ext4", "-q", "-F", "-m", "0", "-E", "nodiscard,lazy_itable_init=0,lazy_journal_init=0", "-L", "homenode-data", "/proc/self/fd/3"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := admitVolume(file, size); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fd := int(parent.Fd())
	if err := unix.Renameat2(fd, stage, fd, name, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return parent.Sync()
}

func admitVolume(file *os.File, size int64) error {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != size {
		return ErrPolicy
	}
	var native unix.Stat_t
	if unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != 0 || native.Nlink != 1 {
		return ErrPolicy
	}
	return nil
}
