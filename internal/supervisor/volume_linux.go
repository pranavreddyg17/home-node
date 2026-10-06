//go:build linux

package supervisor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// New volumes are formatted before exclusive publication. Existing data is never formatted.
func prepareDataVolume(ctx context.Context, path string, size int64, reserve int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || reserve < 4<<30 || reserve > math.MaxInt64-size || size < 16<<20 || size > 512<<30 {
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
	existing, err := root.OpenFile(name, unix.O_PATH|unix.O_NOFOLLOW, 0)
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
	return publishDataVolume(root, file, stage, name, size)
}

func publishDataVolume(root *os.Root, file *os.File, stage, name string, size int64) error {
	if err := admitVolume(file, size); err != nil {
		return err
	}
	prepared, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := root.OpenFile(stage, unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer current.Close()
	if err := admitVolume(current, size); err != nil {
		return err
	}
	observed, err := current.Stat()
	if err != nil || !os.SameFile(prepared, observed) {
		return ErrPolicy
	}
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	fd := int(parent.Fd())
	if err := unix.Renameat2(fd, stage, fd, name, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return parent.Sync()
}

func admitVolume(file *os.File, size int64) error {
	return admitVolumeForUID(file, size, 0)
}

// admitVolumeForUID requires the independently reserved guest identity; it
// neither changes ownership nor proves runtime or restoration authority.
func admitVolumeForUID(file *os.File, size int64, uid uint32) error {
	if file == nil || (uid != 0 && (uid < 65536 || uid > 1<<31-1)) {
		return ErrPolicy
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Size() != size {
		return ErrPolicy
	}
	var native unix.Stat_t
	if unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != uid || native.Nlink != 1 {
		return ErrPolicy
	}
	return nil
}
