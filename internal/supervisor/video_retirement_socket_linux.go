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
	"strings"

	"golang.org/x/sys/unix"
)

// Socket hardlinks by non-owners must remain prohibited while absence can be
// reconciled after interruption. This reads the fixed procfs control; it never
// changes host hardening or treats an ordinary substituted file as policy.
func requireVideoRetirementHardlinks(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile("/proc/sys/fs/protected_hardlinks", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	var fs unix.Statfs_t
	var metadata unix.Stat_t
	if unix.Fstatfs(int(file.Fd()), &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC || unix.Fstat(int(file.Fd()), &metadata) != nil || metadata.Mode&unix.S_IFMT != unix.S_IFREG || metadata.Uid != 0 || metadata.Mode&0022 != 0 {
		return ErrPolicy
	}
	data, err := io.ReadAll(io.LimitReader(file, 16))
	if err != nil || len(data) >= 16 || strings.TrimSpace(string(data)) != "1" {
		return errors.Join(ErrPolicy, err)
	}
	return ctx.Err()
}

// The caller retains runtime/start exclusion and supplies a live stopped/process
// guard. This scope retires only the recorded socket, never its directory,
// volume, ownership records or UID. It is not yet exposed through legacy purge.
func (m *Manager) retireVideoSocket(ctx context.Context, supplied videoRetirementRecord, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || os.Geteuid() != 0 || stopped == nil || !filepath.IsAbs(m.Channels) || filepath.Clean(m.Channels) != m.Channels {
		return record, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return record, err
	}
	if err := requireVideoRetirementHardlinks(ctx); err != nil {
		return record, err
	}
	if err := stopped(ctx); err != nil {
		return record, err
	}
	record = supplied
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		stage, err := m.readVideoRetirementRecordTx(ctx, tx, supplied.Intent)
		if err != nil {
			return err
		}
		if stage != "prepared" && stage != "socket-removing" && stage != "socket-removed" {
			return ErrPolicy
		}
		record.Stage = stage
		return nil
	}); err != nil {
		return record, err
	}
	var active videoRetirementSocket
	count := 0
	for _, entry := range record.Intent.Ownership.Sockets {
		if entry.Retired == 0 {
			active = entry
			count++
		}
	}
	if count != 1 {
		return record, ErrPolicy
	}
	owner := record.Intent.Ownership.Channel
	root, err := os.OpenRoot(m.Channels)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || !m.reservedChannelParentAdmitted(original) {
		return record, ErrPolicy
	}
	directory, err := root.OpenFile(owner.InstanceID, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return record, err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	quarantine := ".adapter-video-retire-" + strconv.FormatInt(record.Intent.Request.Revision, 10)
	name := "adapter.sock"
	var sourceInfo, stageInfo unix.Stat_t
	sourceErr := unix.Fstatat(int(directory.Fd()), name, &sourceInfo, unix.AT_SYMLINK_NOFOLLOW)
	stageErr := unix.Fstatat(int(directory.Fd()), quarantine, &stageInfo, unix.AT_SYMLINK_NOFOLLOW)
	sourceAbsent, stageAbsent := errors.Is(sourceErr, unix.ENOENT), errors.Is(stageErr, unix.ENOENT)
	removed := false
	switch {
	case sourceErr == nil && stageAbsent:
		if record.Stage == "socket-removed" {
			return record, ErrPolicy
		}
	case stageErr == nil && sourceAbsent:
		if record.Stage == "prepared" || record.Stage == "socket-removed" {
			return record, ErrPolicy
		}
		name = quarantine
	case sourceAbsent && stageAbsent:
		if record.Stage == "prepared" {
			return record, ErrPolicy
		}
		removed = true
		name = quarantine
	default:
		return record, errors.Join(ErrPolicy, sourceErr, stageErr)
	}
	var socket *os.File
	if !removed {
		fd, err := unix.Openat(int(directory.Fd()), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return record, err
		}
		socket = os.NewFile(uintptr(fd), name)
		defer func() { result = errors.Join(result, socket.Close()) }()
	}
	allowed := map[string]bool{record.Stage: true}
	check := func(scope context.Context) error {
		if err := scope.Err(); err != nil {
			return err
		}
		if err := requireVideoRetirementHardlinks(scope); err != nil {
			return err
		}
		if err := stopped(scope); err != nil {
			return err
		}
		var currentParent, dir, named unix.Stat_t
		p, pe := parent.Stat()
		pn, pne := os.Lstat(m.Channels)
		if pe != nil || pne != nil || !os.SameFile(p, pn) || unix.Fstat(int(parent.Fd()), &currentParent) != nil || currentParent.Dev != original.Dev || currentParent.Ino != original.Ino || currentParent.Mode != original.Mode || currentParent.Uid != original.Uid || currentParent.Gid != original.Gid || !m.reservedChannelParentAdmitted(currentParent) {
			return ErrPolicy
		}
		for _, entry := range []string{"." + owner.InstanceID + ".channel-prepare", "." + owner.InstanceID + ".video-directory-retire-" + strconv.FormatInt(record.Intent.Request.Revision, 10)} {
			if err := unix.Fstatat(int(parent.Fd()), entry, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
		}
		if unix.Fstat(int(directory.Fd()), &dir) != nil || uint64(dir.Dev) != owner.Device || dir.Ino != owner.Inode || dir.Uid != owner.UID || dir.Gid != owner.AccessGID || dir.Mode != unix.S_IFDIR|0710 || unix.Fstatat(int(parent.Fd()), owner.InstanceID, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != dir.Dev || named.Ino != dir.Ino || named.Mode != dir.Mode || named.Uid != dir.Uid || named.Gid != dir.Gid {
			return ErrPolicy
		}
		if !samePathMount(int(parent.Fd()), unix.AT_FDCWD, m.Channels) || !samePathMount(int(directory.Fd()), int(parent.Fd()), owner.InstanceID) {
			return ErrPolicy
		}
		var pm, dm unix.Statx_t
		if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &pm) != nil || unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &dm) != nil || pm.Mask&unix.STATX_MNT_ID == 0 || dm.Mask&unix.STATX_MNT_ID == 0 || pm.Mnt_id == 0 || pm.Mnt_id != dm.Mnt_id {
			return ErrPolicy
		}
		if socket != nil {
			var child unix.Stat_t
			links := uint64(1)
			if removed {
				links = 0
			}
			if unix.Fstat(int(socket.Fd()), &child) != nil || uint64(child.Dev) != active.Device || child.Ino != active.Inode || child.Uid != owner.UID || child.Gid != owner.AccessGID || child.Mode != unix.S_IFSOCK|0660 || child.Nlink != links {
				return ErrPolicy
			}
			var sm unix.Statx_t
			if unix.Statx(int(socket.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &sm) != nil || sm.Mask&unix.STATX_MNT_ID == 0 || sm.Mnt_id != pm.Mnt_id {
				return ErrPolicy
			}
			if !removed && (unix.Fstatat(int(directory.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != child.Dev || named.Ino != child.Ino || named.Mode != child.Mode || named.Uid != child.Uid || named.Gid != child.Gid || named.Nlink != child.Nlink || !samePathMount(int(socket.Fd()), int(directory.Fd()), name)) {
				return ErrPolicy
			}
		}
		if removed {
			for _, entry := range []string{"adapter.sock", quarantine} {
				if err := unix.Fstatat(int(directory.Fd()), entry, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
					return errors.Join(ErrPolicy, err)
				}
			}
		} else {
			other := quarantine
			if name == quarantine {
				other = "adapter.sock"
			}
			if err := unix.Fstatat(int(directory.Fd()), other, &named, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
				return errors.Join(ErrPolicy, err)
			}
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
		return m.Store.Transaction(scope, func(tx *sql.Tx) error {
			stage, err := m.readVideoRetirementRecordTx(scope, tx, record.Intent)
			if err != nil || !allowed[stage] {
				return errors.Join(ErrPolicy, err)
			}
			return scope.Err()
		})
	}
	if err := check(ctx); err != nil {
		return record, err
	}
	if record.Stage == "prepared" {
		allowed["socket-removing"] = true
		record, err = m.advanceVideoRetirement(ctx, record, "socket-removing", check)
		if err != nil {
			return record, err
		}
	}
	if record.Stage == "socket-removing" {
		allowed = map[string]bool{"socket-removing": true}
		if err := check(ctx); err != nil {
			return record, err
		}
		if !removed && name == "adapter.sock" {
			if err := unix.Renameat2(int(directory.Fd()), name, int(directory.Fd()), quarantine, unix.RENAME_NOREPLACE); err != nil {
				return record, err
			}
			name = quarantine
			if err := directory.Sync(); err != nil {
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
			if err := unix.Unlinkat(int(directory.Fd()), name, 0); err != nil {
				return record, err
			}
			removed = true
		}
		if err := directory.Sync(); err != nil {
			return record, err
		}
		if err := check(ctx); err != nil {
			return record, err
		}
		allowed["socket-removed"] = true
		record, err = m.advanceVideoRetirement(ctx, record, "socket-removed", check)
		if err != nil {
			return record, err
		}
	}
	allowed = map[string]bool{"socket-removed": true}
	if err := check(ctx); err != nil {
		return record, err
	}
	return record, nil
}
