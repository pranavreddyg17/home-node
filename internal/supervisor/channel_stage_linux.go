//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// stageReservedChannel creates only a fresh private directory and durably
// records its inode before any ownership transfer or namespace publication.
// Existing stage/final paths are never adopted. Partial effects are retained.
// The caller must retain runtime exclusion in addition to the live guard.
func (m *Manager) stageReservedChannel(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (intent ChannelOwnershipIntent, result error) {
	defer func() {
		if result != nil {
			intent = ChannelOwnershipIntent{}
		}
	}()
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	if m == nil || stopped == nil || os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath {
		return intent, ErrPolicy
	}
	if err := m.checkChannelPreparationDomain(ctx, d); err != nil {
		return intent, err
	}
	if _, err := m.loadChannelOwnershipIntent(ctx, d); !errors.Is(err, sql.ErrNoRows) {
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
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Uid != 0 || original.Gid != 0 || original.Mode&unix.S_IFMT != unix.S_IFDIR || original.Mode&07022 != 0 {
		return intent, ErrPolicy
	}
	stage := "." + d.ID + ".channel-prepare"
	var directory *os.File
	checkObjects := func() error {
		if _, err := root.Lstat(d.ID); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrPolicy, err)
		}
		var current unix.Stat_t
		opened, openedErr := parent.Stat()
		named, namedErr := os.Lstat(parentPath)
		if openedErr != nil || namedErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, parentPath) {
			return ErrPolicy
		}
		if directory != nil {
			var metadata unix.Stat_t
			if unix.Fstat(int(directory.Fd()), &metadata) != nil || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Mode&unix.S_IFMT != unix.S_IFDIR || metadata.Mode&07777 != 0700 && metadata.Mode&07777 != 0710 {
				return ErrPolicy
			}
			pinned, pinnedErr := directory.Stat()
			named, namedErr := root.Lstat(stage)
			var parentMount, childMount unix.Statx_t
			if pinnedErr != nil || namedErr != nil || !os.SameFile(pinned, named) || !samePathMount(int(directory.Fd()), int(parent.Fd()), stage) || unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != childMount.Mnt_id {
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
		if err := m.checkChannelPreparationDomain(ctx, d); err != nil {
			return err
		}
		return checkObjects()
	}
	if err := guard(); err != nil {
		return intent, err
	}
	for _, name := range []string{stage, d.ID} {
		if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return intent, errors.Join(ErrPolicy, err)
		}
	}
	if err := root.Mkdir(stage, 0700); err != nil {
		return intent, err
	}
	directory, err = root.OpenFile(stage, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	if err := checkObjects(); err != nil {
		return intent, err
	}
	var created unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &created) != nil || created.Uid != 0 || created.Gid != 0 || created.Mode&unix.S_IFMT != unix.S_IFDIR || created.Mode&07777 != 0700 {
		return intent, ErrPolicy
	}
	if err := directory.Chmod(0710); err != nil {
		return intent, err
	}
	if err := directory.Sync(); err != nil {
		return intent, err
	}
	if err := parent.Sync(); err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	intent, err = m.recordPinnedChannelOwnership(ctx, d, directory)
	if err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	verified, err := m.verifyPinnedChannelOwnership(ctx, d, directory)
	if err != nil || verified != intent {
		return intent, errors.Join(ErrPolicy, err)
	}
	return intent, nil
}
