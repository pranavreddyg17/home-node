//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type lockedMetadataCache struct {
	root *os.Root
	lock *os.File
}

// lockMetadataCache uses an already-provisioned private update directory. The
// persistent lock inode lives outside metadata, where TUF must not overwrite
// it. No directory, existing metadata or lock is adopted or repaired here.
func lockMetadataCache(ctx context.Context, provisioned *os.Root) (*lockedMetadataCache, error) {
	if provisioned == nil {
		return nil, errMetadataCache
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := provisioned.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	info, err := directory.Stat()
	var native unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || unix.Fstat(int(directory.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) {
		return nil, errMetadataCache
	}
	lock, err := provisioned.OpenFile("update.lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, errors.Join(errMetadataCache, err)
	}
	retained := false
	defer func() {
		if !retained {
			_ = lock.Close()
		}
	}()
	info, err = lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 || unix.Fstat(int(lock.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
		return nil, errMetadataCache
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(errMetadataCache, err)
	}
	current, err := provisioned.Lstat("update.lock")
	if err != nil || !os.SameFile(info, current) {
		return nil, errMetadataCache
	}
	expected, err := provisioned.Lstat("metadata")
	if err != nil || !expected.IsDir() || expected.Mode().Perm() != 0700 {
		return nil, errMetadataCache
	}
	root, err := provisioned.OpenRoot("metadata")
	if err != nil {
		return nil, err
	}
	metadataDirectory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	observed, err := metadataDirectory.Stat()
	nativeErr := unix.Fstat(int(metadataDirectory.Fd()), &native)
	metadataDirectory.Close()
	if err != nil || nativeErr != nil || !os.SameFile(expected, observed) || native.Uid != uint32(os.Geteuid()) {
		root.Close()
		return nil, errMetadataCache
	}
	if err = ctx.Err(); err != nil {
		root.Close()
		return nil, err
	}
	retained = true
	return &lockedMetadataCache{root: root, lock: lock}, nil
}

// Close releases kernel exclusivity without unlinking its persistent inode.
func (cache *lockedMetadataCache) Close() error {
	return errors.Join(cache.root.Close(), cache.lock.Close())
}
