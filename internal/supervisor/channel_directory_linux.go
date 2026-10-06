//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// prepareGuestChannelDirectory pins the protected parent and child throughout
// ownership changes. Foreign owners and symlinks never authorize mutation.
func prepareGuestChannelDirectory(ctx context.Context, path string, uid, gid int) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || uid < 1 || uid > 1<<31-1 || gid < 1 || gid > 1<<31-1 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrPolicy
	}
	parentPath, name := filepath.Dir(path), filepath.Base(path)
	before, err := os.Lstat(parentPath)
	owner, ok := openedSysUID(before)
	if err != nil || !ok || owner != 0 || !before.IsDir() || before.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	opened, err := parent.Stat()
	owner, ok = openedSysUID(opened)
	if err != nil || !ok || owner != 0 || !os.SameFile(before, opened) || opened.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	if err := root.Mkdir(name, 0710); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	child, err := root.OpenFile(name, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return ErrPolicy
	}
	defer func() { result = errors.Join(result, child.Close()) }()
	original, err := child.Stat()
	if err != nil || original.Mode().Perm()&0022 != 0 || original.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrPolicy
	}
	var native unix.Stat_t
	if unix.Fstat(int(child.Fd()), &native) != nil || (native.Uid != 0 && native.Uid != uint32(uid)) || (native.Uid == 0 && native.Gid != 0) || (native.Uid == uint32(uid) && native.Gid != uint32(gid)) {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := child.Chown(uid, gid); err != nil {
		return err
	}
	if err := child.Chmod(0710); err != nil {
		return err
	}
	if err := child.Sync(); err != nil {
		return err
	}
	info, err := child.Stat()
	current, lookupErr := root.Lstat(name)
	if err != nil || lookupErr != nil || !os.SameFile(info, current) || !current.IsDir() || current.Mode().Perm() != 0710 || unix.Fstat(int(child.Fd()), &native) != nil || native.Uid != uint32(uid) || native.Gid != uint32(gid) {
		return ErrPolicy
	}
	final, err := os.Lstat(parentPath)
	owner, ok = openedSysUID(final)
	if err != nil || !ok || owner != 0 || !os.SameFile(opened, final) || final.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	if err := parent.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}
