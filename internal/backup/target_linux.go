//go:build linux

package backup

import (
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// OpenTarget pins the mounted filesystem with a directory descriptor. Keep this
// descriptor open throughout the operation, and write through relative handles;
// re-resolving MountPath after admission would reintroduce missing-drive fallback.
// RepositoryID must additionally be checked using authenticated restic config
// before any repository mutation; filesystem admission alone is insufficient.
func OpenTarget(t Target) (*os.File, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	identity, err := os.Stat(filepath.Join("/dev/disk/by-uuid", t.UUID))
	if err != nil || identity.Mode()&os.ModeDevice == 0 || identity.Mode()&os.ModeCharDevice != 0 {
		return nil, ErrTarget
	}
	stat, ok := identity.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, ErrTarget
	}
	registered := Device{unix.Major(uint64(stat.Rdev)), unix.Minor(uint64(stat.Rdev))}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	mounts, err := ParseMountInfo(data)
	if err != nil {
		return nil, err
	}
	if _, err = AdmitMount(t, mounts, registered); err != nil {
		return nil, err
	}
	info, err := os.Lstat(t.MountPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrTarget
	}
	directory, err := os.Open(t.MountPath)
	if err != nil {
		return nil, err
	}
	info, err = directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || (Device{unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev))}) != registered {
		_ = directory.Close()
		return nil, ErrTarget
	}
	var filesystem unix.Statfs_t
	if err = unix.Fstatfs(int(directory.Fd()), &filesystem); err != nil || filesystem.Flags&unix.ST_RDONLY != 0 {
		_ = directory.Close()
		return nil, ErrTarget
	}
	return directory, nil
}
