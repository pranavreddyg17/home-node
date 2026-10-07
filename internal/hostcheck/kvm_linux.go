//go:build linux

package hostcheck

import (
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// Linux UAPI: KVM_GET_API_VERSION = _IO(0xAE, 0); stable KVM API = 12.
// This query creates no VM or vCPU and does not prove a guest's device access.
func probeKVMDevice() bool { return probeKVMDeviceAt("/dev/kvm") }

func probeKVMDeviceAt(path string) (usable bool) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	// O_PATH pins metadata without invoking an unrelated device driver's open.
	pinned, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: uint64(unix.O_PATH | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return false
	}
	defer func() {
		if unix.Close(pinned) != nil {
			usable = false
		}
	}()
	var original unix.Stat_t
	if unix.Fstat(pinned, &original) != nil || original.Mode&unix.S_IFMT != unix.S_IFCHR || unix.Major(original.Rdev) != 10 || unix.Minor(original.Rdev) != 232 {
		return false
	}
	// Reopen this process's pinned descriptor, never the original mutable name.
	// The deliberate kernel fd link is accepted only under a retained procfs dir.
	proc, err := unix.Open("/proc/self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer func() {
		if unix.Close(proc) != nil {
			usable = false
		}
	}()
	var filesystem unix.Statfs_t
	if unix.Fstatfs(proc, &filesystem) != nil || filesystem.Type != unix.PROC_SUPER_MAGIC {
		return false
	}
	device, err := unix.Openat(proc, strconv.Itoa(pinned), unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer func() {
		if unix.Close(device) != nil {
			usable = false
		}
	}()
	var opened unix.Stat_t
	if unix.Fstat(device, &opened) != nil || opened.Dev != original.Dev || opened.Ino != original.Ino || opened.Rdev != original.Rdev || opened.Mode&unix.S_IFMT != unix.S_IFCHR {
		return false
	}
	version, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(device), uintptr(0xAE00), 0)
	return errno == 0 && version == 12
}
