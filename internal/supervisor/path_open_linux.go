//go:build linux

package supervisor

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openAbsoluteDirectoryNoLinks walks each canonical component explicitly;
// it never follows an ancestor symlink or relies on an openat2 fallback.
func openAbsoluteDirectoryNoLinks(path string) (result int, resultErr error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, ErrPolicy
	}
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	for _, name := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if name == "" {
			continue
		}
		next, err := unix.Openat(current, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		closeErr := unix.Close(current)
		if err != nil || closeErr != nil {
			if next >= 0 {
				closeErr = errors.Join(closeErr, unix.Close(next))
			}
			return -1, errors.Join(err, closeErr)
		}
		current = next
	}
	return current, nil
}

// openSameMountReadOnly accepts only one component under a retained directory.
// Required mount-ID evidence excludes bind mounts even on the same filesystem.
func openSameMountReadOnly(parent int, name string, directory bool) (int, error) {
	if parent < 0 || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return -1, ErrPolicy
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(parent, name, flags, 0)
	if err != nil {
		return -1, err
	}
	var parentMount, childMount unix.Statx_t
	if unix.Statx(parent, "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &childMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != childMount.Mnt_id {
		return -1, errors.Join(ErrPolicy, unix.Close(fd))
	}
	return fd, nil
}
