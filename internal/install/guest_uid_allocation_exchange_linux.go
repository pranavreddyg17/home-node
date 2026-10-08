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
func exchangeGuestUIDAllocationConfiguration(ctx context.Context, directory *os.File, stage guestUIDAllocationStage, original, desired []byte, guard func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || guard == nil || stage.Version != 1 || stage.Inode == 0 || stage.SourceInode == 0 || stage.Bytes != int64(len(desired)) || len(original) == 0 || len(original) > maxGuestUIDAllocatorConfigurationBytes || len(desired) == 0 || len(desired) > maxGuestUIDAllocatorConfigurationBytes {
		return ErrPlan
	}
	fd := int(directory.Fd())
	var parent unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != 0 || parent.Gid != 0 || parent.Mode&0022 != 0 {
		return ErrConflict
	}
	const pending, final = ".homenode-login-defs.stage", "login.defs"
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
		if unix.Fstat(int(file.Fd()), &stat) != nil || unix.Fstatat(fd, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(stat.Dev) != dev || stat.Ino != inode || named.Dev != stat.Dev || named.Ino != stat.Ino || stat.Nlink != 1 || stat.Uid != 0 || stat.Gid != 0 || stat.Size != int64(len(data)) || (stat.Mode != unix.S_IFREG|0644 && !(allowPrivate && stat.Mode == unix.S_IFREG|0600)) {
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
	if err := replacement.Chmod(0644); err != nil {
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
