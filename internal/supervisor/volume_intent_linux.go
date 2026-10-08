//go:build linux

package supervisor

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
)

// recordPinnedVolumeOwnership derives provenance from the retained descriptor,
// then commits it against independently reserved domain identity. It neither
// changes ownership nor establishes stopped-runtime or pathname authority.
func (m *Manager) recordPinnedVolumeOwnership(ctx context.Context, d Domain, file *os.File) (VolumeOwnershipIntent, error) {
	return m.checkPinnedVolumeOwnership(ctx, d, file, true)
}

// verifyPinnedVolumeOwnership authenticates an existing descriptor's provenance
// without creating an intent. Runtime exclusion and pathname admission remain
// separate requirements for the caller.
func (m *Manager) verifyPinnedVolumeOwnership(ctx context.Context, d Domain, file *os.File) (VolumeOwnershipIntent, error) {
	return m.checkPinnedVolumeOwnership(ctx, d, file, false)
}

func (m *Manager) checkPinnedVolumeOwnership(ctx context.Context, d Domain, file *os.File, create bool) (VolumeOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	if os.Geteuid() != 0 || file == nil || d.GuestUID < 65536 || d.GuestGID == 0 {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	var before unix.Stat_t
	if unix.Fstat(int(file.Fd()), &before) != nil {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	if before.Uid != 0 && before.Uid != d.GuestUID || before.Uid == 0 && before.Gid != 0 || before.Uid == d.GuestUID && before.Gid != d.GuestGID {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	if err := admitVolumeForUID(file, d.Image.DataBytes, before.Uid); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	intent := VolumeOwnershipIntent{InstanceID: d.ID, ImageSHA256: d.Image.SHA256, UID: d.GuestUID, GID: d.GuestGID, Device: uint64(before.Dev), Inode: before.Ino, Size: before.Size}
	check := m.verifyVolumeOwnershipIntent
	if create {
		check = m.recordVolumeOwnershipIntent
	}
	if err := check(ctx, intent); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	var after unix.Stat_t
	if unix.Fstat(int(file.Fd()), &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || before.Nlink != after.Nlink || before.Size != after.Size {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	if err := admitVolumeForUID(file, intent.Size, after.Uid); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	if err := ctx.Err(); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	return intent, nil
}
