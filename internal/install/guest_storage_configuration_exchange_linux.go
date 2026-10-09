//go:build linux

package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Caller retains both durable intent records, a qualified directory and
// allocation/runtime exclusion. Exchange preserves the original under the
// staging name. An interrupted exchange reconciles only both recorded inodes.
func exchangeGuestStorageConfigurationFile(ctx context.Context, directory *os.File, stage guestStorageConfigurationStageFile, original, desired []byte, guard func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || guard == nil || stage.Inode == 0 || stage.SourceInode == 0 || stage.Bytes != int64(len(desired)) || len(original) == 0 || len(original) > 16384 || len(desired) == 0 || len(desired) > 16384 {
		return ErrPlan
	}
	fd := int(directory.Fd())
	var parent unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != 0 || parent.Gid != 0 || parent.Mode&0022 != 0 {
		return ErrConflict
	}
	pending := stage.Name
	final := "services.env"
	mode := uint32(0644)
	if pending == ".homenode-runtime-policy.stage" {
		final = "runtime-policy.json"
		mode = 0600
	} else if pending != ".homenode-services-env.stage" {
		return ErrPlan
	}
	if stage.Device != stage.SourceDevice || stage.Inode == stage.SourceInode || stage.SHA256 != digest(desired) {
		return ErrConflict
	}
	open := func(name string) (*os.File, error) {
		fileFD, err := unix.Openat(fd, name, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fileFD), name), nil
	}
	current, err := open(final)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, current.Close()) }()
	replacement, err := open(pending)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, replacement.Close()) }()
	qualify := func(file *os.File, name string, dev, inode uint64, data []byte, allowPrivate bool) bool {
		var stat, named unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || unix.Fstatat(fd, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(stat.Dev) != dev || stat.Ino != inode || named.Dev != stat.Dev || named.Ino != stat.Ino || stat.Nlink != 1 || stat.Uid != 0 || stat.Gid != 0 || stat.Size != int64(len(data)) || (stat.Mode != unix.S_IFREG|mode && !(allowPrivate && stat.Mode == unix.S_IFREG|0600)) {
			return false
		}
		for _, descriptor := range []*os.File{directory, file} {
			for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(descriptor.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
					return false
				}
			}
		}
		var parentMount, fileMount, namedMount unix.Statx_t
		flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
		if unix.Statx(fd, "", flags, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &fileMount) != nil || unix.Statx(fd, name, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &namedMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || fileMount.Mask&unix.STATX_MNT_ID == 0 || namedMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != fileMount.Mnt_id || fileMount.Mnt_id != namedMount.Mnt_id {
			return false
		}
		contents, err := io.ReadAll(io.NewSectionReader(file, 0, int64(len(data))+1))
		return err == nil && bytes.Equal(contents, data)
	}
	if err := guard(ctx); err != nil {
		return err
	}
	// A completed exchange leaves the new inode at the final name and the
	// exact original at the pending name. Preserve both and finish durability.
	if qualify(current, final, stage.Device, stage.Inode, desired, false) && qualify(replacement, pending, stage.SourceDevice, stage.SourceInode, original, false) {
		if err := directory.Sync(); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if !qualify(current, final, stage.Device, stage.Inode, desired, false) || !qualify(replacement, pending, stage.SourceDevice, stage.SourceInode, original, false) {
			return ErrConflict
		}
		return ctx.Err()
	}
	if !qualify(current, final, stage.SourceDevice, stage.SourceInode, original, false) || !qualify(replacement, pending, stage.Device, stage.Inode, desired, true) {
		return ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replacement.Chmod(os.FileMode(mode)); err != nil {
		return err
	}
	if err := replacement.Sync(); err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if !qualify(current, final, stage.SourceDevice, stage.SourceInode, original, false) || !qualify(replacement, pending, stage.Device, stage.Inode, desired, false) {
		return ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Renameat2(fd, pending, fd, final, unix.RENAME_EXCHANGE); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	if !qualify(replacement, final, stage.Device, stage.Inode, desired, false) || !qualify(current, pending, stage.SourceDevice, stage.SourceInode, original, false) {
		return ErrConflict
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if !qualify(replacement, final, stage.Device, stage.Inode, desired, false) || !qualify(current, pending, stage.SourceDevice, stage.SourceInode, original, false) {
		return ErrConflict
	}
	return ctx.Err()
}
