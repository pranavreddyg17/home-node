//go:build linux

package hostcheck

import (
	"errors"
	"golang.org/x/sys/unix"
)

// Probe actual kernel support in this process, never infer it from a release name.
func probeKernelPaths() (bool, bool) {
	fd, err := unix.Openat2(unix.AT_FDCWD, "/", &unix.OpenHow{Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return false, false
	}
	defer unix.Close(fd)
	var info unix.Statx_t
	err = unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &info)
	return true, err == nil && info.Mask&unix.STATX_MNT_ID != 0 && info.Mnt_id != 0
}

// An invalid descriptor makes this feature probe side-effect-free. EBADF
// proves the kernel accepted AT_EMPTY_PATH; unsupported/blocked calls refuse.
func probeDescriptorChmod() bool {
	return errors.Is(unix.Fchmodat(-1, "", 0600, unix.AT_EMPTY_PATH), unix.EBADF)
}
