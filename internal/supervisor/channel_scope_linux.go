//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// withReservedChannel retains an existing, authenticated empty preparation
// directory and its root-owned parent. Callers must hold runtime exclusion and
// provide a live stopped-guest guard. It never adopts an unrecorded directory.
func (m *Manager) withReservedChannel(ctx context.Context, path string, d Domain, stopped func(context.Context) error, use func(context.Context, *os.File, ChannelOwnershipIntent, func(context.Context) error) error) (result error) {
	return m.withReservedChannelEntry(ctx, path, d, stopped, use, false, nil)
}

// Publication uses the same retained objects and guard as ownership transfer;
// only the exact validated entry name changes after a no-replace rename.
func (m *Manager) withReservedChannelEntry(ctx context.Context, path string, d Domain, stopped func(context.Context) error, use func(context.Context, *os.File, ChannelOwnershipIntent, func(context.Context) error) error, publish bool, guestOwned *bool) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	stage := "." + d.ID + ".channel-prepare"
	expected := d.ID
	if publish {
		expected = stage
	}
	name := filepath.Base(path)
	if m == nil || stopped == nil || use == nil || os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(path) || filepath.Clean(path) != path || name != expected {
		return ErrPolicy
	}
	if err := stopped(ctx); err != nil {
		return err
	}
	parentPath := filepath.Dir(path)
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var originalParent unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &originalParent) != nil || !m.reservedChannelParentAdmitted(originalParent) {
		return ErrPolicy
	}
	directory, err := root.OpenFile(name, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var originalMount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &originalMount) != nil || originalMount.Mask&unix.STATX_MNT_ID == 0 || originalMount.Mnt_id == 0 {
		return ErrPolicy
	}
	intent, err := m.verifyPinnedChannelOwnership(ctx, d, directory)
	if err != nil {
		return err
	}
	checkObjects := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var currentParent unix.Stat_t
		parentInfo, parentErr := parent.Stat()
		parentPathInfo, parentPathErr := os.Lstat(parentPath)
		childInfo, childErr := directory.Stat()
		childPathInfo, childPathErr := root.Lstat(name)
		if parentErr != nil || parentPathErr != nil || childErr != nil || childPathErr != nil || !os.SameFile(parentInfo, parentPathInfo) || !os.SameFile(childInfo, childPathInfo) || unix.Fstat(int(parent.Fd()), &currentParent) != nil || currentParent.Dev != originalParent.Dev || currentParent.Ino != originalParent.Ino || currentParent.Mode != originalParent.Mode || currentParent.Uid != originalParent.Uid || currentParent.Gid != originalParent.Gid {
			return ErrPolicy
		}
		other := stage
		if name == stage {
			other = d.ID
		}
		if _, err := root.Lstat(other); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrPolicy, err)
		}
		var parentMount, parentPathMount, childMount, childPathMount unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(unix.AT_FDCWD, parentPath, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentPathMount) != nil || unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childMount) != nil || unix.Statx(int(parent.Fd()), name, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childPathMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || parentPathMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mask&unix.STATX_MNT_ID == 0 || childPathMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id != originalMount.Mnt_id || parentPathMount.Mnt_id != originalMount.Mnt_id || childMount.Mnt_id != originalMount.Mnt_id || childPathMount.Mnt_id != originalMount.Mnt_id {
			return ErrPolicy
		}
		verified, err := m.verifyPinnedChannelOwnership(ctx, d, directory)
		if err != nil {
			return err
		}
		if verified != intent {
			return ErrPolicy
		}
		if guestOwned != nil && *guestOwned {
			var owner unix.Stat_t
			if unix.Fstat(int(directory.Fd()), &owner) != nil || owner.Uid != intent.UID || owner.Gid != intent.AccessGID {
				return ErrPolicy
			}
		}
		return nil
	}
	guard := func(ctx context.Context) error {
		if err := checkObjects(ctx); err != nil {
			return err
		}
		if err := stopped(ctx); err != nil {
			return err
		}
		return checkObjects(ctx)
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(ctx, directory, intent, guard); err != nil {
		return err
	}
	if publish {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := directory.Sync(); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := unix.Renameat2(int(parent.Fd()), name, int(parent.Fd()), d.ID, unix.RENAME_NOREPLACE); err != nil {
			return err
		}
		name = d.ID
	}
	if err := parent.Sync(); err != nil {
		return err
	}
	return guard(ctx)
}

// transferReservedChannel preserves uncertain ownership effects for an exact
// authenticated retry. It neither creates a socket nor enables a guest.
func (m *Manager) transferReservedChannel(ctx context.Context, path string, d Domain, stopped func(context.Context) error) (ChannelOwnershipIntent, error) {
	var transferred ChannelOwnershipIntent
	completed := false
	err := m.withReservedChannelEntry(ctx, path, d, stopped, func(ctx context.Context, directory *os.File, intent ChannelOwnershipIntent, guard func(context.Context) error) error {
		if err := guard(ctx); err != nil {
			return err
		}
		var before unix.Stat_t
		if unix.Fstat(int(directory.Fd()), &before) != nil || uint64(before.Dev) != intent.Device || before.Ino != intent.Inode || before.Uid != 0 && before.Uid != intent.UID || before.Uid == 0 && before.Gid != 0 || before.Uid == intent.UID && before.Gid != intent.AccessGID {
			return ErrPolicy
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if before.Uid == 0 {
			if err := directory.Chown(int(intent.UID), int(intent.AccessGID)); err != nil {
				return err
			}
		}
		if err := directory.Sync(); err != nil {
			return err
		}
		completed = true
		if err := guard(ctx); err != nil {
			return err
		}
		transferred = intent
		return nil
	}, false, &completed)
	if err != nil {
		return ChannelOwnershipIntent{}, err
	}
	return transferred, nil
}
