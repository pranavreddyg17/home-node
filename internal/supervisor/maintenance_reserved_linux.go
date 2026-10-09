//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// The caller retains exclusive live-manager maintenance access throughout.
func (m *Manager) withReservedMaintenanceDisk(ctx context.Context, token string, instance Instance, copyDisk func(context.Context, *os.File, Instance) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil || copyDisk == nil {
		return ErrPolicy
	}
	image, err := m.Manifest.Image(instance.Workload)
	if err != nil || image.SHA256 != instance.ImageSHA256 || image.DataBytes != instance.DataBytes {
		return errors.Join(ErrPolicy, err)
	}
	d := Domain{ID: instance.ID, Image: image}
	if err := m.bindDomainGuestIdentity(ctx, &d, false); err != nil {
		return err
	}
	intent, err := m.loadMaintenanceVolumeIntent(ctx, token, d, instance.Revision)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(m.Volumes)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil {
		return ErrPolicy
	}
	file, err := openReservedMaintenanceVolume(ctx, m.Volumes, d)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	objects := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current, disk, named unix.Stat_t
		if unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != 0 || current.Gid != d.GuestGID || current.Mode&07777 != 0710 {
			return ErrPolicy
		}
		opened, err := parent.Stat()
		pathInfo, pathErr := os.Lstat(m.Volumes)
		if err != nil || pathErr != nil || !os.SameFile(opened, pathInfo) || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, m.Volumes) {
			return errors.Join(ErrPolicy, err, pathErr)
		}
		if err := admitVolumeForUID(file, intent.Size, intent.UID); err != nil {
			return err
		}
		flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
		if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
			return errors.Join(ErrPolicy, err)
		}
		if unix.Fstat(int(file.Fd()), &disk) != nil || uint64(disk.Dev) != intent.Device || disk.Ino != intent.Inode || disk.Gid != intent.GID || unix.Fstatat(int(parent.Fd()), d.ID+".raw", &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != disk.Dev || named.Ino != disk.Ino || named.Mode != disk.Mode || named.Uid != disk.Uid || named.Gid != disk.Gid || named.Nlink != disk.Nlink || named.Size != disk.Size || !samePathMount(int(file.Fd()), int(parent.Fd()), d.ID+".raw") {
			return ErrPolicy
		}
		var pm, dm unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &dm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || dm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 || pm.Mnt_id != dm.Mnt_id {
			return ErrPolicy
		}
		return ctx.Err()
	}
	guard := func() error {
		if err := objects(); err != nil {
			return err
		}
		observed, err := m.loadMaintenanceVolumeIntent(ctx, token, d, instance.Revision)
		if err != nil || observed != intent {
			return errors.Join(ErrPolicy, err)
		}
		running, err := m.Backend.Running(ctx, d.ID)
		if err != nil || running {
			return errors.Join(ErrPolicy, err)
		}
		processes, err := ObserveGuestUIDProcessConflicts(ctx, *m.GuestUIDPool)
		if err != nil || processes.Blocked[d.GuestUID] {
			return errors.Join(ErrPolicy, err)
		}
		return objects()
	}
	if err := guard(); err != nil {
		return err
	}
	if err := copyDisk(ctx, file, instance); err != nil {
		return err
	}
	return guard()
}
