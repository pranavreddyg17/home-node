//go:build linux

package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// openReservedSystemImage admits only the protected root:guest-group image
// inode and verifies its catalog digest through a retained read-only descriptor.
// This does not authenticate the catalog signature or publish storage policy.
func openReservedSystemImage(ctx context.Context, directory string, d Domain) (result *os.File, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.Image.Bytes < 1 || d.Image.Bytes > 64<<30 || len(d.Image.SHA256) != 64 || strings.Trim(d.Image.SHA256, "0123456789abcdef") != "" || os.Geteuid() != 0 || !guestproto.ValidID(d.ID) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 {
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
	name := d.Image.SHA256 + ".raw"
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
	if unix.Fstat(fd, &disk) != nil || disk.Uid != 0 || disk.Gid != d.GuestGID {
		return nil, ErrPolicy
	}
	if err := admitReservedSystemImage(metadata, d); err != nil {
		return nil, err
	}
	// Only the qualified regular inode may acquire read access. The
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
	reopened, err := unix.Openat(proc, strconv.Itoa(fd), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(reopened), name)
	fail := func(err error) (*os.File, error) { return nil, errors.Join(err, file.Close()) }
	var ioDisk unix.Stat_t
	if unix.Fstat(reopened, &ioDisk) != nil || disk.Dev != ioDisk.Dev || disk.Ino != ioDisk.Ino || disk.Mode != ioDisk.Mode || disk.Uid != ioDisk.Uid || disk.Gid != ioDisk.Gid || disk.Nlink != ioDisk.Nlink || disk.Size != ioDisk.Size {
		return fail(ErrPolicy)
	}
	if err := admitReservedSystemImage(file, d); err != nil {
		return fail(err)
	}
	if err := verifyReservedSystemImageDigest(ctx, file, d); err != nil {
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
	var verified unix.Stat_t
	if unix.Fstat(reopened, &verified) != nil || verified.Dev != disk.Dev || verified.Ino != disk.Ino || verified.Mode != disk.Mode || verified.Uid != disk.Uid || verified.Gid != disk.Gid || verified.Nlink != disk.Nlink || verified.Size != disk.Size {
		return fail(ErrPolicy)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return file, nil
}

func admitReservedSystemImage(file *os.File, d Domain) error {
	var disk unix.Stat_t
	if file == nil || unix.Fstat(int(file.Fd()), &disk) != nil || disk.Uid != 0 || disk.Gid != d.GuestGID || disk.Mode&unix.S_IFMT != unix.S_IFREG || disk.Mode&07777 != 0440 || disk.Nlink != 1 || disk.Size != d.Image.Bytes {
		return ErrPolicy
	}
	return nil
}

func verifyReservedSystemImageDigest(ctx context.Context, file *os.File, d Domain) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	buffer := make([]byte, 128<<10)
	reader := io.LimitReader(file, d.Image.Bytes+1)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
			size += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if size != d.Image.Bytes || hex.EncodeToString(hash.Sum(nil)) != d.Image.SHA256 {
		return ErrPolicy
	}
	_, err := file.Seek(0, io.SeekStart)
	return err
}
