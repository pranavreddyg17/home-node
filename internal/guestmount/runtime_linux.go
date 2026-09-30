//go:build linux

package guestmount

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// CheckRuntime verifies the visible temporary OS mounts without changing them.
// It does not prove the underlying signed system image or total VM memory use.
func CheckRuntime() error {
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return ErrMount
	}
	info, err := io.ReadAll(io.LimitReader(mounts, (1<<20)+1))
	mounts.Close()
	if err != nil || len(info) > 1<<20 {
		return ErrMount
	}
	devices := map[uint64]bool{}
	for _, item := range []struct {
		path string
		mode uint32
	}{{"/tmp", 01777}, {"/var", 0755}} {
		device, err := checkRuntimePath(item.path, item.mode, string(info))
		if err != nil || devices[device] {
			return ErrMount
		}
		devices[device] = true
	}
	return nil
}

func checkRuntimePath(path string, mode uint32, mounts string) (uint64, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, ErrMount
	}
	defer unix.Close(fd)
	var identity unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &identity) != nil || unix.Fstatfs(fd, &fs) != nil || identity.Uid != 0 || identity.Gid != 0 || identity.Mode&07777 != mode {
		return 0, ErrMount
	}
	if admitRuntimeFS(fs) != nil || (path == "/tmp" && fs.Flags&unix.ST_RDONLY != 0) {
		return 0, ErrMount
	}
	mountID, err := descriptorMountID(fd)
	if err != nil {
		return 0, ErrMount
	}
	device := fmt.Sprintf("%d:%d", unix.Major(identity.Dev), unix.Minor(identity.Dev))
	if admitRuntimeMount(mounts, path, device, mountID) != nil {
		return 0, ErrMount
	}
	return identity.Dev, nil
}

func admitRuntimeFS(fs unix.Statfs_t) error {
	required := int64(unix.ST_NODEV | unix.ST_NOSUID | unix.ST_NOEXEC)
	if fs.Type != unix.TMPFS_MAGIC || fs.Flags&required != required || fs.Bsize <= 0 || fs.Blocks == 0 || fs.Blocks > (64<<20)/uint64(fs.Bsize) || fs.Files == 0 || fs.Files > 8192 {
		return ErrMount
	}
	return nil
}
