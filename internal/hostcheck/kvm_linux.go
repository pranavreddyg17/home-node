//go:build linux

package hostcheck

import (
	"golang.org/x/sys/unix"
	"path/filepath"
)

// Linux UAPI: KVM_GET_API_VERSION = _IO(0xAE, 0); stable KVM API = 12.
// This query creates no VM or vCPU and does not prove a guest's device access.
func probeKVMDevice() bool { return probeKVMDeviceAt("/dev/kvm") }

func probeKVMDeviceAt(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: uint64(unix.O_RDWR | unix.O_CLOEXEC | unix.O_NONBLOCK), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return false
	}
	var info unix.Stat_t
	valid := unix.Fstat(fd, &info) == nil && info.Mode&unix.S_IFMT == unix.S_IFCHR
	if valid {
		version, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(0xAE00), 0)
		valid = errno == 0 && version == 12
	}
	return unix.Close(fd) == nil && valid
}
