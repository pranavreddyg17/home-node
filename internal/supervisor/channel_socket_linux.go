//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// recordPinnedChannelSocket derives provenance from retained descriptors after
// separate live peer and domain verification. It grants no adoption, runtime
// exclusion, namespace publication, or unlink authority.
func (m *Manager) recordPinnedChannelSocket(ctx context.Context, d Domain, revision int64, directory, socket *os.File) (ChannelSocketIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelSocketIntent{}, err
	}
	if m == nil || directory == nil || socket == nil || os.Geteuid() != 0 {
		return ChannelSocketIntent{}, ErrPolicy
	}
	channel, err := m.loadChannelOwnershipIntent(ctx, d)
	if err != nil {
		return ChannelSocketIntent{}, err
	}
	return m.checkPinnedChannelSocket(ctx, revision, channel, directory, socket, true)
}

func (m *Manager) checkPinnedChannelSocket(ctx context.Context, revision int64, channel ChannelOwnershipIntent, directory, socket *os.File, create bool) (ChannelSocketIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelSocketIntent{}, err
	}
	if m == nil || directory == nil || socket == nil || os.Geteuid() != 0 {
		return ChannelSocketIntent{}, ErrPolicy
	}
	var originalDirectory, originalSocket unix.Stat_t
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var parent, child, named unix.Stat_t
		if unix.Fstat(int(directory.Fd()), &parent) != nil || uint64(parent.Dev) != channel.Device || parent.Ino != channel.Inode || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Mode&07777 != 0710 || parent.Uid != channel.UID || parent.Gid != channel.AccessGID || unix.Fstat(int(socket.Fd()), &child) != nil || child.Mode&unix.S_IFMT != unix.S_IFSOCK || child.Mode&07777 != 0660 || child.Uid != channel.UID || child.Gid != channel.AccessGID || child.Nlink != 1 || unix.Fstatat(int(directory.Fd()), "adapter.sock", &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != child.Dev || named.Ino != child.Ino || named.Mode != child.Mode || named.Uid != child.Uid || named.Gid != child.Gid || named.Nlink != child.Nlink {
			return ErrPolicy
		}
		var parentMount, childMount unix.Statx_t
		if !samePathMount(int(socket.Fd()), int(directory.Fd()), "adapter.sock") || unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(socket.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != childMount.Mnt_id {
			return ErrPolicy
		}
		if _, err := directory.Seek(0, io.SeekStart); err != nil {
			return err
		}
		entries, err := directory.ReadDir(2)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(entries) != 1 || entries[0].Name() != "adapter.sock" {
			return ErrPolicy
		}
		if originalSocket.Ino != 0 && (child.Dev != originalSocket.Dev || child.Ino != originalSocket.Ino || child.Mode != originalSocket.Mode || child.Uid != originalSocket.Uid || child.Gid != originalSocket.Gid || child.Nlink != originalSocket.Nlink || parent.Dev != originalDirectory.Dev || parent.Ino != originalDirectory.Ino || parent.Mode != originalDirectory.Mode || parent.Uid != originalDirectory.Uid || parent.Gid != originalDirectory.Gid) {
			return ErrPolicy
		}
		originalDirectory, originalSocket = parent, child
		return nil
	}
	if err := check(); err != nil {
		return ChannelSocketIntent{}, err
	}
	intent := ChannelSocketIntent{Channel: channel, Revision: revision, Device: uint64(originalSocket.Dev), Inode: originalSocket.Ino}
	if err := m.checkChannelSocketIntent(ctx, intent, create); err != nil {
		return ChannelSocketIntent{}, err
	}
	if err := check(); err != nil {
		return ChannelSocketIntent{}, err
	}
	if err := m.verifyChannelSocketIntent(ctx, intent); err != nil {
		return ChannelSocketIntent{}, err
	}
	if err := check(); err != nil {
		return ChannelSocketIntent{}, err
	}
	return intent, nil
}
