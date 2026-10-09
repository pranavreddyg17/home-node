//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// stageReservedVolume creates only a fresh private disk and durably
// records its inode before any ownership transfer or namespace publication.
// Existing stage/final paths are never adopted. Partial effects are retained.
// The caller must retain runtime exclusion in addition to the live guard.
func (m *Manager) stageReservedVolume(ctx context.Context, parentPath string, d Domain, reserve int64, stopped func(context.Context) error) (intent VolumeOwnershipIntent, result error) {
	defer func() {
		if result != nil {
			intent = VolumeOwnershipIntent{}
		}
	}()
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	if d.Image.DataBytes < 16<<20 || d.Image.DataBytes > 512<<30 || reserve < 4<<30 || reserve > math.MaxInt64-d.Image.DataBytes || m == nil || stopped == nil || os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return intent, ErrPolicy
	}
	if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
		return intent, err
	}
	if _, err := m.loadVolumeOwnershipIntent(ctx, d); !errors.Is(err, sql.ErrNoRows) {
		return intent, errors.Join(ErrPolicy, err)
	}
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Uid != 0 || original.Gid != d.GuestGID || original.Mode&unix.S_IFMT != unix.S_IFDIR || original.Mode&07777 != 0710 {
		return intent, ErrPolicy
	}
	stage := "." + d.ID + ".volume-prepare"
	var file *os.File
	checkObjects := func() error {
		var current unix.Stat_t
		opened, openedErr := parent.Stat()
		named, namedErr := os.Lstat(parentPath)
		if openedErr != nil || namedErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, parentPath) {
			return ErrPolicy
		}
		if file != nil {
			var metadata unix.Stat_t
			if unix.Fstat(int(file.Fd()), &metadata) != nil || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Mode&unix.S_IFMT != unix.S_IFREG || metadata.Mode&07777 != 0600 || metadata.Nlink != 1 {
				return ErrPolicy
			}
			pinned, pinnedErr := file.Stat()
			named, namedErr := root.Lstat(stage)
			var parentMount, childMount unix.Statx_t
			if pinnedErr != nil || namedErr != nil || !os.SameFile(pinned, named) || !samePathMount(int(file.Fd()), int(parent.Fd()), stage) || unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != childMount.Mnt_id {
				return ErrPolicy
			}
		}
		return ctx.Err()
	}
	guard := func() error {
		if err := checkObjects(); err != nil {
			return err
		}
		if err := stopped(ctx); err != nil {
			return err
		}
		if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
			return err
		}
		return checkObjects()
	}
	if err := guard(); err != nil {
		return intent, err
	}
	for _, name := range []string{stage, d.ID + ".raw"} {
		if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return intent, errors.Join(ErrPolicy, err)
		}
	}
	var space unix.Statfs_t
	if unix.Fstatfs(int(parent.Fd()), &space) != nil || space.Bsize <= 0 {
		return intent, ErrCapacity
	}
	blocks := (uint64(d.Image.DataBytes+reserve) + uint64(space.Bsize) - 1) / uint64(space.Bsize)
	if space.Bavail < blocks {
		return intent, ErrCapacity
	}
	file, err = root.OpenFile(stage, os.O_RDWR|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	if err := checkObjects(); err != nil {
		return intent, err
	}
	var created unix.Stat_t
	if unix.Fstat(int(file.Fd()), &created) != nil || created.Uid != 0 || created.Gid != 0 || created.Mode&unix.S_IFMT != unix.S_IFREG || created.Mode&07777 != 0600 || created.Nlink != 1 || created.Size != 0 {
		return intent, ErrPolicy
	}
	if err := file.Truncate(d.Image.DataBytes); err != nil {
		return intent, err
	}
	if err := unix.Fallocate(int(file.Fd()), 0, 0, d.Image.DataBytes); err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	if err := admitVolume(file, d.Image.DataBytes); err != nil {
		return intent, err
	}
	if _, err := commandWithFiles(ctx, "", []*os.File{file}, "/usr/sbin/mkfs.ext4", "-q", "-F", "-m", "0", "-E", "nodiscard,lazy_itable_init=0,lazy_journal_init=0", "-L", "homenode-data", "/proc/self/fd/3"); err != nil {
		return intent, err
	}
	if err := file.Sync(); err != nil {
		return intent, err
	}
	if err := admitVolume(file, d.Image.DataBytes); err != nil {
		return intent, err
	}
	if err := parent.Sync(); err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	intent, err = m.recordPinnedVolumeOwnership(ctx, d, file)
	if err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	verified, err := m.verifyPinnedVolumeOwnership(ctx, d, file)
	if err != nil || verified != intent {
		return intent, errors.Join(ErrPolicy, err)
	}
	return intent, nil
}
