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
	admitted, err := AdmitMount(t, mounts, registered)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(t.MountPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrTarget
	}
	directory, err := openObservedTargetDirectory(t.MountPath, info)
	if err != nil {
		return nil, ErrTarget
	}
	info, err = directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, ErrTarget
	}
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || (Device{unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev))}) != registered {
		_ = directory.Close()
		return nil, ErrTarget
	}
	if err := requireTargetMountID(directory, admitted.ID); err != nil {
		directory.Close()
		return nil, err
	}
	var filesystem unix.Statfs_t
	if err = unix.Fstatfs(int(directory.Fd()), &filesystem); err != nil || filesystem.Flags&unix.ST_RDONLY != 0 {
		_ = directory.Close()
		return nil, ErrTarget
	}
	return directory, nil
}

// Resolve every component without symlinks, including proc-style magic links.
// Supported Linux hosts must provide openat2; no weaker fallback is permitted.
func openTargetDirectory(path string) (*os.File, error) {
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "registered-backup-target"), nil
}

func requireTargetMountID(directory *os.File, expected uint64) error {
	if directory == nil || expected == 0 {
		return ErrTarget
	}
	var info unix.Statx_t
	if err := unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &info); err != nil || info.Mask&unix.STATX_MNT_ID == 0 || info.Mnt_id != expected {
		return ErrTarget
	}
	return nil
}

func openObservedTargetDirectory(path string, expected os.FileInfo) (*os.File, error) {
	if expected == nil || !expected.IsDir() {
		return nil, ErrTarget
	}
	file, err := openTargetDirectory(path)
	if err != nil {
		return nil, ErrTarget
	}
	observed, err := file.Stat()
	if err != nil || !os.SameFile(expected, observed) {
		file.Close()
		return nil, ErrTarget
	}
	return file, nil
}
