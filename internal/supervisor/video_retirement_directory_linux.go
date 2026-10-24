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

// The caller retains runtime/start exclusion. This scope removes only the
// recorded empty channel directory and keeps all provenance and the UID lease.
func (m *Manager) retireVideoDirectory(ctx context.Context, supplied videoRetirementRecord, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || os.Geteuid() != 0 || stopped == nil || !filepath.IsAbs(m.Channels) || filepath.Clean(m.Channels) != m.Channels || !filepath.IsAbs(m.Volumes) || filepath.Clean(m.Volumes) != m.Volumes {
		return record, ErrPolicy
	}
	record = supplied
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		phase, err := m.readVideoRetirementRecordTx(ctx, tx, record.Intent)
		if err != nil {
			return err
		}
		if phase != "volume-removed" && phase != "directory-removing" && phase != "directory-removed" {
			return ErrPolicy
		}
		record.Stage = phase
		return nil
	}); err != nil {
		return record, err
	}
	owner := record.Intent.Ownership.Channel
	parent, err := os.OpenFile(m.Channels, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	volumeParent, err := os.OpenFile(m.Volumes, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, volumeParent.Close()) }()
	var original, volumeOriginal unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || !m.reservedChannelParentAdmitted(original) || unix.Fstat(int(volumeParent.Fd()), &volumeOriginal) != nil || volumeOriginal.Mode != unix.S_IFDIR|0710 || volumeOriginal.Uid != 0 || volumeOriginal.Gid != record.Intent.Ownership.Volume.GID {
		return record, ErrPolicy
	}
	source := owner.InstanceID
	quarantine := "." + source + ".video-directory-retire-" + strconv.FormatInt(record.Intent.Request.Revision, 10)
	var a, b unix.Stat_t
	ae := unix.Fstatat(int(parent.Fd()), source, &a, unix.AT_SYMLINK_NOFOLLOW)
	be := unix.Fstatat(int(parent.Fd()), quarantine, &b, unix.AT_SYMLINK_NOFOLLOW)
	name := source
	removed := false
	switch {
	case ae == nil && errors.Is(be, unix.ENOENT):
		if record.Stage == "directory-removed" {
			return record, ErrPolicy
		}
	case be == nil && errors.Is(ae, unix.ENOENT):
		if record.Stage != "directory-removing" {
			return record, ErrPolicy
		}
		name = quarantine
	case errors.Is(ae, unix.ENOENT) && errors.Is(be, unix.ENOENT):
		if record.Stage == "volume-removed" {
			return record, ErrPolicy
		}
		removed = true
		name = quarantine
	default:
		return record, errors.Join(ErrPolicy, ae, be)
	}
	var directory *os.File
	if !removed {
		fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return record, errors.Join(ErrPolicy, err)
		}
		directory = os.NewFile(uintptr(fd), name)
		defer func() { result = errors.Join(result, directory.Close()) }()
	}
	allowed := map[string]bool{record.Stage: true}
	check := func(scope context.Context) error {
		if err := requireVideoRetirementHardlinks(scope); err != nil {
			return err
		}
		if err := stopped(scope); err != nil {
			return err
		}
		for _, p := range []struct {
			file     *os.File
			path     string
			original unix.Stat_t
		}{{parent, m.Channels, original}, {volumeParent, m.Volumes, volumeOriginal}} {
			opened, oe := p.file.Stat()
			named, ne := os.Lstat(p.path)
			var now unix.Stat_t
			if oe != nil || ne != nil || !os.SameFile(opened, named) || unix.Fstat(int(p.file.Fd()), &now) != nil || now.Dev != p.original.Dev || now.Ino != p.original.Ino || now.Mode != p.original.Mode || now.Uid != p.original.Uid || now.Gid != p.original.Gid || !samePathMount(int(p.file.Fd()), unix.AT_FDCWD, p.path) {
				return ErrPolicy
			}
		}
		for _, entry := range []string{"." + source + ".volume-prepare", source + ".raw", "." + source + ".video-volume-retire-" + strconv.FormatInt(record.Intent.Request.Revision, 10)} {
			var v unix.Stat_t
			if err := unix.Fstatat(int(volumeParent.Fd()), entry, &v, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		}
		var preparation unix.Stat_t
		if err := unix.Fstatat(int(parent.Fd()), "."+source+".channel-prepare", &preparation, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
			return errors.Join(ErrPolicy, err)
		}
		var pm unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 {
			return ErrPolicy
		}
		if directory != nil {
			var dir, named unix.Stat_t
			links := uint64(2)
			if removed {
				links = 0
			}
			if unix.Fstat(int(directory.Fd()), &dir) != nil || uint64(dir.Dev) != owner.Device || dir.Ino != owner.Inode || dir.Mode != unix.S_IFDIR|0710 || dir.Uid != owner.UID || dir.Gid != owner.AccessGID || dir.Nlink != links {
				return ErrPolicy
			}
			var dm unix.Statx_t
			if unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &dm) != nil || dm.Mask&unix.STATX_MNT_ID == 0 || dm.Mnt_id != pm.Mnt_id {
				return ErrPolicy
			}
			if !removed {
				if unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != dir.Dev || named.Ino != dir.Ino || named.Mode != dir.Mode || named.Uid != dir.Uid || named.Gid != dir.Gid || named.Nlink != dir.Nlink || !samePathMount(int(directory.Fd()), int(parent.Fd()), name) {
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
			}
		}
		entries := []string{source, quarantine}
		if !removed {
			if name == source {
				entries = []string{quarantine}
			} else {
				entries = []string{source}
			}
		}
		for _, entry := range entries {
			var named unix.Stat_t
			if err := unix.Fstatat(int(parent.Fd()), entry, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		}
		return m.Store.Transaction(scope, func(tx *sql.Tx) error {
			phase, err := m.readVideoRetirementRecordTx(scope, tx, record.Intent)
			if err != nil || !allowed[phase] {
				return errors.Join(ErrPolicy, err)
			}
			return scope.Err()
		})
	}
	if err := check(ctx); err != nil {
		return record, err
	}
	if record.Stage == "volume-removed" {
		allowed["directory-removing"] = true
		record, err = m.advanceVideoRetirement(ctx, record, "directory-removing", check)
		if err != nil {
			return record, err
		}
	}
	if record.Stage == "directory-removing" {
		allowed = map[string]bool{"directory-removing": true}
		if err := check(ctx); err != nil {
			return record, err
		}
		if !removed && name == source {
			if err := unix.Renameat2(int(parent.Fd()), name, int(parent.Fd()), quarantine, unix.RENAME_NOREPLACE); err != nil {
				return record, err
			}
			name = quarantine
			if err := parent.Sync(); err != nil {
				return record, err
			}
			if err := check(ctx); err != nil {
				return record, err
			}
		}
		if !removed {
			if err := check(ctx); err != nil {
				return record, err
			}
			if err := unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR); err != nil {
				return record, err
			}
			removed = true
		}
		if err := parent.Sync(); err != nil {
			return record, err
		}
		if err := check(ctx); err != nil {
			return record, err
		}
		allowed["directory-removed"] = true
		record, err = m.advanceVideoRetirement(ctx, record, "directory-removed", check)
		if err != nil {
			return record, err
		}
	}
	allowed = map[string]bool{"directory-removed": true}
	if err := check(ctx); err != nil {
		return record, err
	}
	return record, nil
}
