//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// Retain the recorded empty channel while retiring its volume. A journal phase
// alone is insufficient: a recreated socket or foreign entry must stop cleanup.
func (m *Manager) withVideoVolumeChannel(ctx context.Context, intent videoRetirementIntent, stopped func(context.Context) error, use func(func(context.Context) error) error) (result error) {
	if m == nil || m.Store == nil || stopped == nil || use == nil || os.Geteuid() != 0 || !filepath.IsAbs(m.Channels) || filepath.Clean(m.Channels) != m.Channels {
		return ErrPolicy
	}
	owner := intent.Ownership.Channel
	root, err := os.OpenRoot(m.Channels)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || !m.reservedChannelParentAdmitted(original) {
		return ErrPolicy
	}
	directory, err := root.OpenFile(owner.InstanceID, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	check := func(scope context.Context) error {
		if err := requireVideoRetirementHardlinks(scope); err != nil {
			return err
		}
		if err := stopped(scope); err != nil {
			return err
		}
		var current, dir, named unix.Stat_t
		opened, oe := parent.Stat()
		path, pe := os.Lstat(m.Channels)
		if oe != nil || pe != nil || !os.SameFile(opened, path) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || !m.reservedChannelParentAdmitted(current) {
			return ErrPolicy
		}
		for _, entry := range []string{"." + owner.InstanceID + ".channel-prepare", "." + owner.InstanceID + ".video-directory-retire-" + strconv.FormatInt(intent.Request.Revision, 10)} {
			if err := unix.Fstatat(int(parent.Fd()), entry, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		}
		if unix.Fstat(int(directory.Fd()), &dir) != nil || uint64(dir.Dev) != owner.Device || dir.Ino != owner.Inode || dir.Uid != owner.UID || dir.Gid != owner.AccessGID || dir.Mode != unix.S_IFDIR|0710 || unix.Fstatat(int(parent.Fd()), owner.InstanceID, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != dir.Dev || named.Ino != dir.Ino || named.Mode != dir.Mode || named.Uid != dir.Uid || named.Gid != dir.Gid || named.Nlink != dir.Nlink {
			return ErrPolicy
		}
		if !samePathMount(int(parent.Fd()), unix.AT_FDCWD, m.Channels) || !samePathMount(int(directory.Fd()), int(parent.Fd()), owner.InstanceID) {
			return ErrPolicy
		}
		var pm, dm unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &dm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || dm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 || pm.Mnt_id != dm.Mnt_id {
			return ErrPolicy
		}
		if _, err := directory.Seek(0, io.SeekStart); err != nil {
			return err
		}
		entries, err := directory.ReadDir(1)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(entries) != 0 {
			return ErrPolicy
		}
		return m.Store.Transaction(scope, func(tx *sql.Tx) error {
			stage, err := m.readVideoRetirementRecordTx(scope, tx, intent)
			if err != nil {
				return err
			}
			if stage != "socket-removed" && stage != "volume-removing" && stage != "volume-removed" {
				return ErrPolicy
			}
			return scope.Err()
		})
	}
	if err := check(ctx); err != nil {
		return err
	}
	if err := use(check); err != nil {
		return err
	}
	return check(ctx)
}

// The caller retains runtime/start exclusion. O_PATH pins the volume without
// reading guest bytes or opening a substituted device for I/O. This operation
// never removes the channel directory, provenance or reserved UID lease.
func (m *Manager) retireVideoVolume(ctx context.Context, supplied videoRetirementRecord, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || stopped == nil || os.Geteuid() != 0 || !filepath.IsAbs(m.Volumes) || filepath.Clean(m.Volumes) != m.Volumes {
		return record, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return record, err
	}
	record = supplied
	result = m.withVideoVolumeChannel(ctx, record.Intent, stopped, func(channelGuard func(context.Context) error) (result error) {
		if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
			stage, err := m.readVideoRetirementRecordTx(ctx, tx, record.Intent)
			if err != nil {
				return err
			}
			if stage != "socket-removed" && stage != "volume-removing" && stage != "volume-removed" {
				return ErrPolicy
			}
			record.Stage = stage
			return nil
		}); err != nil {
			return err
		}
		owner := record.Intent.Ownership.Volume
		root, err := os.OpenRoot(m.Volumes)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, parent.Close()) }()
		var original unix.Stat_t
		if unix.Fstat(int(parent.Fd()), &original) != nil || original.Uid != 0 || original.Gid != owner.GID || original.Mode != unix.S_IFDIR|0710 {
			return ErrPolicy
		}
		source := owner.InstanceID + ".raw"
		quarantine := "." + owner.InstanceID + ".video-volume-retire-" + strconv.FormatInt(record.Intent.Request.Revision, 10)
		var sourceInfo, stageInfo unix.Stat_t
		sourceErr := unix.Fstatat(int(parent.Fd()), source, &sourceInfo, unix.AT_SYMLINK_NOFOLLOW)
		stageErr := unix.Fstatat(int(parent.Fd()), quarantine, &stageInfo, unix.AT_SYMLINK_NOFOLLOW)
		sourceAbsent, stageAbsent := errors.Is(sourceErr, unix.ENOENT), errors.Is(stageErr, unix.ENOENT)
		name := source
		removed := false
		switch {
		case sourceErr == nil && stageAbsent:
			if record.Stage == "volume-removed" {
				return ErrPolicy
			}
		case stageErr == nil && sourceAbsent:
			if record.Stage != "volume-removing" {
				return ErrPolicy
			}
			name = quarantine
		case sourceAbsent && stageAbsent:
			if record.Stage == "socket-removed" {
				return ErrPolicy
			}
			removed = true
			name = quarantine
		default:
			return errors.Join(ErrPolicy, sourceErr, stageErr)
		}
		var file *os.File
		if !removed {
			fd, err := unix.Openat(int(parent.Fd()), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			file = os.NewFile(uintptr(fd), name)
			defer func() { result = errors.Join(result, file.Close()) }()
		}
		allowed := map[string]bool{record.Stage: true}
		check := func(scope context.Context) error {
			if err := channelGuard(scope); err != nil {
				return err
			}
			var current, named unix.Stat_t
			opened, oe := parent.Stat()
			path, pe := os.Lstat(m.Volumes)
			if oe != nil || pe != nil || !os.SameFile(opened, path) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid || current.Uid != 0 || current.Gid != owner.GID || current.Mode != unix.S_IFDIR|0710 || !samePathMount(int(parent.Fd()), unix.AT_FDCWD, m.Volumes) {
				return ErrPolicy
			}
			if err := unix.Fstatat(int(parent.Fd()), "."+owner.InstanceID+".volume-prepare", &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
			var pm unix.Statx_t
			if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 {
				return ErrPolicy
			}
			if file != nil {
				var disk unix.Stat_t
				links := uint64(1)
				if removed {
					links = 0
				}
				flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
				if err != nil || flags&unix.O_PATH == 0 || unix.Fstat(int(file.Fd()), &disk) != nil || uint64(disk.Dev) != owner.Device || disk.Ino != owner.Inode || disk.Uid != owner.UID || disk.Gid != owner.GID || disk.Mode != unix.S_IFREG|0600 || disk.Nlink != links || disk.Size != owner.Size {
					return ErrPolicy
				}
				var dm unix.Statx_t
				if unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &dm) != nil || dm.Mask&unix.STATX_MNT_ID == 0 || dm.Mnt_id != pm.Mnt_id {
					return ErrPolicy
				}
				if !removed && (unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != disk.Dev || named.Ino != disk.Ino || named.Uid != disk.Uid || named.Gid != disk.Gid || named.Mode != disk.Mode || named.Nlink != disk.Nlink || named.Size != disk.Size || !samePathMount(int(file.Fd()), int(parent.Fd()), name)) {
					return ErrPolicy
				}
			}
			if removed {
				for _, entry := range []string{source, quarantine} {
					if err := unix.Fstatat(int(parent.Fd()), entry, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
						return errors.Join(ErrPolicy, err)
					}
				}
			} else {
				other := quarantine
				if name == quarantine {
					other = source
				}
				if err := unix.Fstatat(int(parent.Fd()), other, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
					return errors.Join(ErrPolicy, err)
				}
			}
			return m.Store.Transaction(scope, func(tx *sql.Tx) error {
				stage, err := m.readVideoRetirementRecordTx(scope, tx, record.Intent)
				if err != nil || !allowed[stage] {
					return errors.Join(ErrPolicy, err)
				}
				return scope.Err()
			})
		}
		if err := check(ctx); err != nil {
			return err
		}
		if record.Stage == "socket-removed" {
			allowed["volume-removing"] = true
			record, err = m.advanceVideoRetirement(ctx, record, "volume-removing", check)
			if err != nil {
				return err
			}
		}
		if record.Stage == "volume-removing" {
			allowed = map[string]bool{"volume-removing": true}
			if err := check(ctx); err != nil {
				return err
			}
			if !removed && name == source {
				if err := unix.Renameat2(int(parent.Fd()), name, int(parent.Fd()), quarantine, unix.RENAME_NOREPLACE); err != nil {
					return err
				}
				name = quarantine
				if err := parent.Sync(); err != nil {
					return err
				}
				if err := check(ctx); err != nil {
					return err
				}
			}
			if !removed {
				if err := check(ctx); err != nil {
					return err
				}
				if err := unix.Unlinkat(int(parent.Fd()), name, 0); err != nil {
					return err
				}
				removed = true
			}
			if err := parent.Sync(); err != nil {
				return err
			}
			if err := check(ctx); err != nil {
				return err
			}
			allowed["volume-removed"] = true
			record, err = m.advanceVideoRetirement(ctx, record, "volume-removed", check)
			if err != nil {
				return err
			}
		}
		allowed = map[string]bool{"volume-removed": true}
		return check(ctx)
	})
	return record, result
}
