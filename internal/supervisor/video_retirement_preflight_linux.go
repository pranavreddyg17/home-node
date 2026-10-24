//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// Before any socket effect, retain and admit the volume that will be retired
// next. O_PATH never reads guest bytes or opens a substituted device for I/O.
// The caller still owns runtime/start exclusion and process-exit qualification.
func (m *Manager) retireVideoSocketWithVolume(ctx context.Context, supplied videoRetirementRecord, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || stopped == nil || os.Geteuid() != 0 || !filepath.IsAbs(m.Volumes) || filepath.Clean(m.Volumes) != m.Volumes {
		return record, ErrPolicy
	}
	if err := stopped(ctx); err != nil {
		return record, err
	}
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		phase, err := m.readVideoRetirementRecordTx(ctx, tx, supplied.Intent)
		if err != nil {
			return err
		}
		if phase != "prepared" && phase != "socket-removing" && phase != "socket-removed" {
			return ErrPolicy
		}
		return ctx.Err()
	}); err != nil {
		return record, err
	}
	owner := supplied.Intent.Ownership.Volume
	parent, err := os.OpenFile(m.Volumes, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Uid != 0 || original.Gid != owner.GID || original.Mode != unix.S_IFDIR|0710 {
		return record, ErrPolicy
	}
	name := owner.InstanceID + ".raw"
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return record, errors.Join(ErrPolicy, err)
	}
	volume := os.NewFile(uintptr(fd), name)
	defer func() { result = errors.Join(result, volume.Close()) }()
	check := func(scope context.Context) error {
		if err := scope.Err(); err != nil {
			return err
		}
		if err := stopped(scope); err != nil {
			return err
		}
		opened, oe := parent.Stat()
		named, ne := os.Lstat(m.Volumes)
		var current, disk, path unix.Stat_t
		if oe != nil || ne != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, m.Volumes) {
			return ErrPolicy
		}
		if unix.Fstat(int(volume.Fd()), &disk) != nil || uint64(disk.Dev) != owner.Device || disk.Ino != owner.Inode || disk.Uid != owner.UID || disk.Gid != owner.GID || disk.Mode != unix.S_IFREG|0600 || disk.Nlink != 1 || disk.Size != owner.Size {
			return ErrPolicy
		}
		if unix.Fstatat(int(parent.Fd()), name, &path, unix.AT_SYMLINK_NOFOLLOW) != nil || path.Dev != disk.Dev || path.Ino != disk.Ino || path.Mode != disk.Mode || path.Uid != disk.Uid || path.Gid != disk.Gid || path.Nlink != disk.Nlink || path.Size != disk.Size || !samePathMount(int(volume.Fd()), int(parent.Fd()), name) {
			return ErrPolicy
		}
		var pm, vm unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || unix.Statx(int(volume.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &vm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || vm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 || pm.Mnt_id != vm.Mnt_id {
			return ErrPolicy
		}
		for _, other := range []string{"." + owner.InstanceID + ".volume-prepare", "." + owner.InstanceID + ".video-volume-retire-" + strconv.FormatInt(supplied.Intent.Request.Revision, 10)} {
			if err := unix.Fstatat(int(parent.Fd()), other, &path, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		}
		return m.Store.Transaction(scope, func(tx *sql.Tx) error {
			phase, err := m.readVideoRetirementRecordTx(scope, tx, supplied.Intent)
			if err != nil {
				return err
			}
			if phase != "prepared" && phase != "socket-removing" && phase != "socket-removed" {
				return ErrPolicy
			}
			return scope.Err()
		})
	}
	if err := check(ctx); err != nil {
		return record, err
	}
	record, err = m.retireVideoSocket(ctx, supplied, check)
	if err != nil {
		return record, err
	}
	if err := check(ctx); err != nil {
		return record, err
	}
	return record, nil
}
