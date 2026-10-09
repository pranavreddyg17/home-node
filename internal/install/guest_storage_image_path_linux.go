//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Caller retains and qualifies the installation's image parent pathname and
// migration authority. This keeps one image open and rejects pathname or mount
// replacement. Image content and ownership are qualified by the consumer.
func withGuestStorageImagePath(ctx context.Context, root *os.Root, parent *os.File, sha string, checkParent func(context.Context) error, use func(*os.File, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if root == nil || parent == nil || checkParent == nil || use == nil || len(sha) != 64 || strings.Trim(sha, "0123456789abcdef") != "" {
		return ErrPlan
	}
	if err := checkParent(ctx); err != nil {
		return err
	}
	name := sha + ".raw"
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(file.Fd()), &original) != nil || original.Mode&unix.S_IFMT != unix.S_IFREG || original.Nlink != 1 {
		return ErrConflict
	}
	guard := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkParent(ctx); err != nil {
			return err
		}
		var current, named unix.Stat_t
		if unix.Fstat(int(file.Fd()), &current) != nil || unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != original.Dev || current.Ino != original.Ino || named.Dev != current.Dev || named.Ino != current.Ino || named.Mode != current.Mode || named.Uid != current.Uid || named.Gid != current.Gid || named.Nlink != 1 || named.Size != current.Size {
			return ErrConflict
		}
		var parentMount, fileMount, namedMount unix.Statx_t
		flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
		if unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &fileMount) != nil || unix.Statx(int(parent.Fd()), name, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &namedMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || fileMount.Mask&unix.STATX_MNT_ID == 0 || namedMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || fileMount.Mnt_id != parentMount.Mnt_id || namedMount.Mnt_id != fileMount.Mnt_id {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(file, guard); err != nil {
		return err
	}
	return guard(ctx)
}
