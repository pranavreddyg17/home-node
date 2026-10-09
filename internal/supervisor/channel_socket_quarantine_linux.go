//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// quarantineChannelSocket moves only the durable active socket inode to a
// fixed revision-derived name. Callers retain runtime exclusion throughout.
// This preserves interrupted effects and neither unlinks nor marks retirement complete.
func (m *Manager) quarantineChannelSocket(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (ChannelSocketIntent, error) {
	return m.withRetiringChannelSocket(ctx, parentPath, d, stopped, false, nil)
}

// removeQuarantinedChannelSocket removes only the journaled quarantined inode.
// An absent retry is admitted only under its durable retirement checkpoint.
// Completion publication remains separate from this physical, synced removal.
func (m *Manager) removeQuarantinedChannelSocket(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error) (ChannelSocketIntent, error) {
	return m.withRetiringChannelSocket(ctx, parentPath, d, stopped, true, nil)
}

// retireChannelSocket completes only the explicitly selected historical record.
// A completed retry requires both socket names absent under the same directory.
func (m *Manager) retireChannelSocket(ctx context.Context, parentPath string, d Domain, expected ChannelSocketIntent, stopped func(context.Context) error) (ChannelSocketIntent, error) {
	return m.withRetiringChannelSocket(ctx, parentPath, d, stopped, true, &expected)
}

func (m *Manager) withRetiringChannelSocket(ctx context.Context, parentPath string, d Domain, stopped func(context.Context) error, remove bool, expected *ChannelSocketIntent) (intent ChannelSocketIntent, result error) {
	defer func() {
		if result != nil {
			intent = ChannelSocketIntent{}
		}
	}()
	if !filepath.IsAbs(parentPath) || filepath.Clean(parentPath) != parentPath || d.ChannelPath != filepath.Join(parentPath, d.ID, "adapter.sock") || os.Geteuid() != 0 {
		return intent, ErrPolicy
	}
	var saved ChannelSocketIntent
	var err error
	completed := false
	if expected != nil {
		if stopped == nil {
			return intent, ErrPolicy
		}
		if err := stopped(ctx); err != nil {
			return intent, err
		}
		saved, completed, err = m.loadChannelSocketRetirementRecord(ctx, d, expected.Revision, true)
		if err != nil || saved != *expected {
			return intent, errors.Join(ErrPolicy, err)
		}
	}
	if !completed {
		saved, err = m.beginChannelSocketRetirement(ctx, d, stopped)
		if err != nil || expected != nil && saved != *expected {
			return intent, errors.Join(ErrPolicy, err)
		}
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
	directory, err := root.OpenFile(d.ID, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	childRoot, err := os.OpenRoot(filepath.Dir(d.ChannelPath))
	if err != nil {
		return intent, err
	}
	defer func() { result = errors.Join(result, childRoot.Close()) }()
	stage := ".adapter-retire-" + strconv.FormatInt(saved.Revision, 10)
	name, publish := "adapter.sock", true
	removed := false
	_, sourceErr := childRoot.Lstat(name)
	_, stageErr := childRoot.Lstat(stage)
	if sourceErr == nil && errors.Is(stageErr, os.ErrNotExist) {
	} else if stageErr == nil && errors.Is(sourceErr, os.ErrNotExist) {
		name, publish = stage, false
	} else if remove && errors.Is(stageErr, os.ErrNotExist) && errors.Is(sourceErr, os.ErrNotExist) {
		name, publish, removed = stage, false, true
	} else {
		return intent, errors.Join(ErrPolicy, sourceErr, stageErr)
	}
	if completed && !removed {
		return intent, ErrPolicy
	}
	var socket *os.File
	if !removed {
		socket, err = childRoot.OpenFile(name, unix.O_PATH|unix.O_NOFOLLOW, 0)
		if err != nil {
			return intent, err
		}
		defer func() { result = errors.Join(result, socket.Close()) }()
	}
	var originalMount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &originalMount) != nil || originalMount.Mask&unix.STATX_MNT_ID == 0 || originalMount.Mnt_id == 0 {
		return intent, ErrPolicy
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var currentParent, dir, child unix.Stat_t
		parentInfo, parentErr := parent.Stat()
		parentPathInfo, parentPathErr := os.Lstat(parentPath)
		dirInfo, dirErr := directory.Stat()
		dirPathInfo, dirPathErr := root.Lstat(d.ID)
		if parentErr != nil || parentPathErr != nil || dirErr != nil || dirPathErr != nil || !os.SameFile(parentInfo, parentPathInfo) || !os.SameFile(dirInfo, dirPathInfo) || unix.Fstat(int(parent.Fd()), &currentParent) != nil || currentParent.Dev != original.Dev || currentParent.Ino != original.Ino || currentParent.Mode != original.Mode || currentParent.Uid != original.Uid || currentParent.Gid != original.Gid || unix.Fstat(int(directory.Fd()), &dir) != nil || uint64(dir.Dev) != saved.Channel.Device || dir.Ino != saved.Channel.Inode || dir.Uid != saved.Channel.UID || dir.Gid != saved.Channel.AccessGID || dir.Mode&unix.S_IFMT != unix.S_IFDIR || dir.Mode&07777 != 0710 {
			return ErrPolicy
		}
		if socket != nil {
			expectedLinks := uint64(1)
			if removed {
				expectedLinks = 0
			}
			if unix.Fstat(int(socket.Fd()), &child) != nil || uint64(child.Dev) != saved.Device || child.Ino != saved.Inode || child.Uid != saved.Channel.UID || child.Gid != saved.Channel.AccessGID || child.Mode&unix.S_IFMT != unix.S_IFSOCK || child.Mode&07777 != 0660 || child.Nlink != expectedLinks {
				return ErrPolicy
			}
		}
		if removed {
			if _, err := childRoot.Lstat(name); !errors.Is(err, os.ErrNotExist) {
				return errors.Join(ErrPolicy, err)
			}
			var named unix.Stat_t
			if err := unix.Fstatat(int(directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		} else {
			childInfo, childErr := socket.Stat()
			childPathInfo, childPathErr := childRoot.Lstat(name)
			var named unix.Stat_t
			if childErr != nil || childPathErr != nil || !os.SameFile(childInfo, childPathInfo) || unix.Fstatat(int(directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != child.Dev || named.Ino != child.Ino || named.Mode != child.Mode || named.Uid != child.Uid || named.Gid != child.Gid || named.Nlink != child.Nlink {
				return ErrPolicy
			}
		}
		other := stage
		if name == stage {
			other = "adapter.sock"
		}
		if _, err := childRoot.Lstat(other); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrPolicy, err)
		}
		if _, err := directory.Seek(0, io.SeekStart); err != nil {
			return err
		}
		entries, err := directory.ReadDir(2)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if removed && len(entries) != 0 || !removed && (len(entries) != 1 || entries[0].Name() != name) {
			return ErrPolicy
		}
		if !samePathMount(int(parent.Fd()), unix.AT_FDCWD, parentPath) || !samePathMount(int(directory.Fd()), int(parent.Fd()), d.ID) || !removed && !samePathMount(int(socket.Fd()), int(directory.Fd()), name) {
			return ErrPolicy
		}
		for _, file := range []*os.File{parent, directory, socket} {
			if file == nil {
				continue
			}
			var mount unix.Statx_t
			if unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &mount) != nil || mount.Mask&unix.STATX_MNT_ID == 0 || mount.Mnt_id != originalMount.Mnt_id {
				return ErrPolicy
			}
		}
		verified, retired, err := m.loadChannelSocketRetirementRecord(ctx, d, saved.Revision, expected != nil)
		if err != nil || verified != saved || retired != completed {
			return errors.Join(ErrPolicy, err)
		}
		var started int
		if err := m.Store.DB.QueryRowContext(ctx, `SELECT retirement_started FROM runtime_channel_sockets WHERE instance_id=? AND revision=?`, d.ID, saved.Revision).Scan(&started); err != nil || started != 1 {
			return errors.Join(ErrPolicy, err)
		}
		return nil
	}
	guard := func() error {
		if err := check(); err != nil {
			return err
		}
		if err := stopped(ctx); err != nil {
			return err
		}
		return check()
	}
	if err := guard(); err != nil {
		return intent, err
	}
	if publish {
		if err := ctx.Err(); err != nil {
			return intent, err
		}
		if err := unix.Renameat2(int(directory.Fd()), name, int(directory.Fd()), stage, unix.RENAME_NOREPLACE); err != nil {
			return intent, err
		}
		name = stage
	}
	if err := directory.Sync(); err != nil {
		return intent, err
	}
	if err := guard(); err != nil {
		return intent, err
	}
	if remove && !removed {
		if err := ctx.Err(); err != nil {
			return intent, err
		}
		if err := unix.Unlinkat(int(directory.Fd()), name, 0); err != nil {
			return intent, err
		}
		removed = true
		if err := directory.Sync(); err != nil {
			return intent, err
		}
		if err := guard(); err != nil {
			return intent, err
		}
	}
	if expected != nil && !completed {
		if !removed {
			return intent, ErrPolicy
		}
		if err := m.publishChannelSocketRetirement(ctx, d, saved); err != nil {
			return intent, err
		}
		completed = true
		if err := guard(); err != nil {
			return intent, err
		}
	}
	return saved, nil
}
