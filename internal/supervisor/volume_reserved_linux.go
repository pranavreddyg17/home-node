//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// openReservedVolume admits the proposed root:guest-group 0710 volume parent
// and retains the exact regular disk. It does not authenticate durable intent
// or exclude a running guest; callers must establish both before mutation.
func openReservedVolume(ctx context.Context, directory string, d Domain) (*os.File, error) {
	return openReservedVolumeEntry(ctx, directory, d, false)
}

// Staged admission uses only the fixed instance-derived preparation name and
// the same descriptor, mount and metadata checks as a published disk. It does
// not adopt the inode or authorize formatting or namespace publication.
func openReservedVolumeEntry(ctx context.Context, directory string, d Domain, staged bool) (result *os.File, resultErr error) {
	return openReservedVolumeAccess(ctx, directory, d, staged, false)
}

// Maintenance access is read-only and requires completed guest ownership.
func openReservedMaintenanceVolume(ctx context.Context, directory string, d Domain) (*os.File, error) {
	return openReservedVolumeAccess(ctx, directory, d, false, true)
}

func openReservedVolumeAccess(ctx context.Context, directory string, d Domain, staged, readOnly bool) (result *os.File, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.Image.DataBytes < 16<<20 || d.Image.DataBytes > 512<<30 || os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 {
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
	if staged {
		name = "." + d.ID + ".volume-prepare"
	}
	// name is exactly one validated instance-derived component. O_NOFOLLOW
	// pins symlinks themselves, which regular-inode admission rejects. Require
	// mount IDs rather than device equality, so bind mounts also refuse.
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	metadata := os.NewFile(uintptr(fd), name)
	defer func() { resultErr = errors.Join(resultErr, metadata.Close()) }()
	var parentMount, diskMount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &diskMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || diskMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id == 0 || parentMount.Mnt_id != diskMount.Mnt_id {
		return nil, ErrPolicy
	}
	var disk unix.Stat_t
	if unix.Fstat(fd, &disk) != nil || disk.Uid != 0 && disk.Uid != d.GuestUID || disk.Uid == 0 && disk.Gid != 0 || disk.Uid == d.GuestUID && disk.Gid != d.GuestGID {
		return nil, ErrPolicy
	}
	if readOnly && disk.Uid != d.GuestUID {
		return nil, ErrPolicy
	}
	if err := admitVolumeForUID(metadata, d.Image.DataBytes, disk.Uid); err != nil {
		return nil, err
	}
	// Only the qualified regular inode may acquire read/write access. The
	// kernel descriptor link is deliberate, under a retained procfs directory.
	proc, err := unix.Open("/proc/self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(proc)) }()
	var filesystem unix.Statfs_t
	if unix.Fstatfs(proc, &filesystem) != nil || filesystem.Type != unix.PROC_SUPER_MAGIC {
		return nil, ErrPolicy
	}
	access := unix.O_RDWR
	if readOnly {
		access = unix.O_RDONLY
	}
	reopened, err := unix.Openat(proc, strconv.Itoa(fd), access|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(reopened), name)
	fail := func(err error) (*os.File, error) { return nil, errors.Join(err, file.Close()) }
	var ioDisk unix.Stat_t
	if unix.Fstat(reopened, &ioDisk) != nil || disk.Dev != ioDisk.Dev || disk.Ino != ioDisk.Ino || disk.Mode != ioDisk.Mode || disk.Uid != ioDisk.Uid || disk.Gid != ioDisk.Gid || disk.Nlink != ioDisk.Nlink || disk.Size != ioDisk.Size {
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
	if unix.Fstat(reopened, &finalDisk) != nil || disk.Dev != finalDisk.Dev || disk.Ino != finalDisk.Ino || disk.Mode != finalDisk.Mode || disk.Uid != finalDisk.Uid || disk.Gid != finalDisk.Gid || disk.Nlink != finalDisk.Nlink || disk.Size != finalDisk.Size {
		return fail(ErrPolicy)
	}
	var finalParentMount, finalDiskMount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &finalParentMount) != nil || unix.Statx(reopened, "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &finalDiskMount) != nil || finalParentMount.Mask&unix.STATX_MNT_ID == 0 || finalDiskMount.Mask&unix.STATX_MNT_ID == 0 || finalParentMount.Mnt_id != parentMount.Mnt_id || finalDiskMount.Mnt_id != diskMount.Mnt_id {
		return fail(ErrPolicy)
	}
	// A bind mount can preserve device/inode while changing the current path's
	// mount. Recheck path mount identity as well as retained descriptor identity.
	var pathParentMount, pathDiskMount unix.Statx_t
	if unix.Statx(unix.AT_FDCWD, directory, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &pathParentMount) != nil || unix.Statx(int(parent.Fd()), name, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &pathDiskMount) != nil || pathParentMount.Mask&unix.STATX_MNT_ID == 0 || pathDiskMount.Mask&unix.STATX_MNT_ID == 0 || pathParentMount.Mnt_id != parentMount.Mnt_id || pathDiskMount.Mnt_id != diskMount.Mnt_id {
		return fail(ErrPolicy)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return file, nil
}
