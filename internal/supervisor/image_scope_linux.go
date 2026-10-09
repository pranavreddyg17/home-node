//go:build linux

package supervisor

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

// withReservedSystemImage retains the authenticated image and parent across
// preparation. Metadata guards avoid repeatedly hashing large images; content
// is verified on admission and again after the consumer's final runtime check.
func withReservedSystemImage(ctx context.Context, directory string, d Domain, stopped func(context.Context) error, use func(context.Context, *os.File, func(context.Context) error) error) (result error) {
	if stopped == nil || use == nil {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := stopped(ctx); err != nil {
		return err
	}
	image, err := openReservedSystemImage(ctx, directory, d)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, image.Close()) }()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var originalParent, originalImage unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &originalParent) != nil || originalParent.Uid != 0 || originalParent.Gid != d.GuestGID || originalParent.Mode&unix.S_IFMT != unix.S_IFDIR || originalParent.Mode&07777 != 0710 || unix.Fstat(int(image.Fd()), &originalImage) != nil {
		return ErrPolicy
	}
	var mount unix.Statx_t
	if unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &mount) != nil || mount.Mask&unix.STATX_MNT_ID == 0 || mount.Mnt_id == 0 {
		return ErrPolicy
	}
	check := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var currentParent, currentImage unix.Stat_t
		parentInfo, parentErr := parent.Stat()
		parentPathInfo, parentPathErr := os.Lstat(directory)
		imageInfo, imageErr := image.Stat()
		imagePathInfo, imagePathErr := root.Lstat(d.Image.SHA256 + ".raw")
		if parentErr != nil || parentPathErr != nil || imageErr != nil || imagePathErr != nil || !os.SameFile(parentInfo, parentPathInfo) || !os.SameFile(imageInfo, imagePathInfo) || unix.Fstat(int(parent.Fd()), &currentParent) != nil || currentParent.Dev != originalParent.Dev || currentParent.Ino != originalParent.Ino || currentParent.Mode != originalParent.Mode || currentParent.Uid != originalParent.Uid || currentParent.Gid != originalParent.Gid || unix.Fstat(int(image.Fd()), &currentImage) != nil || currentImage.Dev != originalImage.Dev || currentImage.Ino != originalImage.Ino || currentImage.Mode != originalImage.Mode || currentImage.Uid != originalImage.Uid || currentImage.Gid != originalImage.Gid || currentImage.Nlink != originalImage.Nlink || currentImage.Size != originalImage.Size {
			return ErrPolicy
		}
		var parentMount, imageMount unix.Statx_t
		if !samePathMount(int(parent.Fd()), unix.AT_FDCWD, directory) || !samePathMount(int(image.Fd()), int(parent.Fd()), d.Image.SHA256+".raw") || unix.Statx(int(parent.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &parentMount) != nil || unix.Statx(int(image.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_DONT_SYNC, unix.STATX_MNT_ID, &imageMount) != nil || parentMount.Mask&unix.STATX_MNT_ID == 0 || imageMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mnt_id != mount.Mnt_id || imageMount.Mnt_id != mount.Mnt_id {
			return ErrPolicy
		}
		return admitReservedSystemImage(image, d)
	}
	guard := func(ctx context.Context) error {
		if err := check(ctx); err != nil {
			return err
		}
		if err := stopped(ctx); err != nil {
			return err
		}
		return check(ctx)
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(ctx, image, guard); err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := verifyReservedSystemImageDigest(ctx, image, d); err != nil {
		return err
	}
	return check(ctx)
}
