//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// These helpers derive directory provenance from a retained descriptor and
// authenticate its independently reserved identity. They require an empty
// preparation directory; pathname admission and stopped-runtime exclusion
// remain prerequisites for any subsequent mutation.
func (m *Manager) recordPinnedChannelOwnership(ctx context.Context, d Domain, directory *os.File) (ChannelOwnershipIntent, error) {
	return m.checkPinnedChannelOwnership(ctx, d, directory, true)
}

func (m *Manager) verifyPinnedChannelOwnership(ctx context.Context, d Domain, directory *os.File) (ChannelOwnershipIntent, error) {
	return m.checkPinnedChannelOwnership(ctx, d, directory, false)
}

func (m *Manager) checkPinnedChannelOwnership(ctx context.Context, d Domain, directory *os.File, create bool) (ChannelOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	if m == nil || directory == nil || os.Geteuid() != 0 {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	accessGID, err := m.channelAccessGID()
	if err != nil {
		return ChannelOwnershipIntent{}, err
	}
	var before unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFDIR || before.Mode&07777 != 0710 || before.Nlink < 2 || before.Uid != 0 && before.Uid != d.GuestUID || before.Uid == 0 && before.Gid != 0 || before.Uid == d.GuestUID && before.Gid != accessGID {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	checkEmpty := func() error {
		if _, err := directory.Seek(0, io.SeekStart); err != nil {
			return err
		}
		entries, err := directory.ReadDir(1)
		if len(entries) != 0 || err != nil && !errors.Is(err, io.EOF) {
			return errors.Join(ErrPolicy, err)
		}
		return nil
	}
	if err := checkEmpty(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	intent := ChannelOwnershipIntent{InstanceID: d.ID, ImageSHA256: d.Image.SHA256, UID: d.GuestUID, GuestGID: d.GuestGID, AccessGID: accessGID, Device: uint64(before.Dev), Inode: before.Ino}
	check := m.verifyChannelOwnershipIntent
	if create {
		check = m.recordChannelOwnershipIntent
	}
	if err := check(ctx, intent); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	if err := checkEmpty(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	var after unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || before.Nlink != after.Nlink || before.Size != after.Size {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	return intent, nil
}
