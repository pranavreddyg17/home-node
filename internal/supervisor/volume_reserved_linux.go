//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// openReservedVolume admits the proposed root:guest-group 0710 volume parent
// and retains the exact regular disk. It does not authenticate durable intent
// or exclude a running guest; callers must establish both before mutation.
func openReservedVolume(ctx context.Context, directory string, d Domain) (result *os.File, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 {
		return nil, ErrPolicy
	}
	before, err := os.Lstat(directory)
	if err != nil || !before.IsDir() || before.Mode().Perm() != 0710 || before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, ErrPolicy
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, root.Close())
		if resultErr != nil && result != nil {
			resultErr = errors.Join(resultErr, result.Close())
			result = nil
		}
	}()
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	opened, err := parent.Stat()
	var policy unix.Stat_t
	if err != nil || !os.SameFile(before, opened) || unix.Fstat(int(parent.Fd()), &policy) != nil || policy.Uid != 0 || policy.Gid != d.GuestGID || policy.Mode&07777 != 0710 {
		return nil, ErrPolicy
	}
	name := d.ID + ".raw"
	fd, err := unix.Openat2(int(parent.Fd()), name, &unix.OpenHow{Flags: unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	fail := func(err error) (*os.File, error) { return nil, errors.Join(err, file.Close()) }
	var disk unix.Stat_t
	if unix.Fstat(fd, &disk) != nil || disk.Uid != 0 && disk.Uid != d.GuestUID || disk.Uid == 0 && disk.Gid != 0 || disk.Uid == d.GuestUID && disk.Gid != d.GuestGID {
		return fail(ErrPolicy)
	}
	if err := admitVolumeForUID(file, d.Image.DataBytes, disk.Uid); err != nil {
		return fail(err)
	}
	pinned, err := file.Stat()
	current, pathErr := root.Lstat(name)
	currentParent, parentErr := os.Lstat(directory)
	var finalParent unix.Stat_t
	if err != nil || pathErr != nil || parentErr != nil || !os.SameFile(pinned, current) || !os.SameFile(before, currentParent) || currentParent.Mode() != before.Mode() || unix.Fstat(int(parent.Fd()), &finalParent) != nil || policy.Dev != finalParent.Dev || policy.Ino != finalParent.Ino || policy.Mode != finalParent.Mode || policy.Uid != finalParent.Uid || policy.Gid != finalParent.Gid {
		return fail(ErrPolicy)
	}
	var finalDisk unix.Stat_t
	if unix.Fstat(fd, &finalDisk) != nil || disk.Dev != finalDisk.Dev || disk.Ino != finalDisk.Ino || disk.Mode != finalDisk.Mode || disk.Uid != finalDisk.Uid || disk.Gid != finalDisk.Gid || disk.Nlink != finalDisk.Nlink || disk.Size != finalDisk.Size {
		return fail(ErrPolicy)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return file, nil
}
