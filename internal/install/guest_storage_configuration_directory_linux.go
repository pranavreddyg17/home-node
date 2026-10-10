//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Bind a caller-retained descriptor to this installation's configuration
// directory. Descriptor metadata alone cannot establish namespace authority.
func (e *Engine) checkGuestStorageConfigurationDirectory(ctx context.Context, directory *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || directory == nil {
		return ErrPlan
	}
	var metadata unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &metadata) != nil || metadata.Mode&unix.S_IFMT != unix.S_IFDIR || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Mode&07022 != 0 {
		return ErrConflict
	}
	opened, err := directory.Stat()
	named, nameErr := e.host.Lstat("etc/homenode")
	if err != nil || nameErr != nil || !os.SameFile(opened, named) {
		return ErrConflict
	}
	var retainedMount, pathMount unix.Statx_t
	if unix.Statx(int(directory.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &retainedMount) != nil || unix.Statx(unix.AT_FDCWD, filepath.Join(e.host.Name(), "etc/homenode"), unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &pathMount) != nil || retainedMount.Mask&unix.STATX_MNT_ID == 0 || pathMount.Mask&unix.STATX_MNT_ID == 0 || retainedMount.Mnt_id == 0 || retainedMount.Mnt_id != pathMount.Mnt_id {
		return ErrConflict
	}
	for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		if _, err := unix.Fgetxattr(int(directory.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
			return ErrConflict
		}
	}
	return ctx.Err()
}
