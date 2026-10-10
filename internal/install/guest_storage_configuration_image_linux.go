//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"golang.org/x/sys/unix"
)

// Verify the catalog digest through the same retained root:guest-group inode
// whose path and mount are checked. A matching digest never substitutes for DAC.
func (e *Engine) verifyGuestStorageConfigurationImage(ctx context.Context, image catalog.Image, gid uint32, guard func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || image.Bytes < 1 || image.Bytes > 64<<30 || gid == 0 || gid > 1<<31-1 {
		return ErrPlan
	}
	root, err := e.host.OpenRoot("var/lib/homenode/images")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	checkParent := func(ctx context.Context) error {
		if err := guard(ctx); err != nil {
			return err
		}
		opened, err := parent.Stat()
		named, pathErr := e.host.Lstat("var/lib/homenode/images")
		var metadata unix.Stat_t
		if err != nil || pathErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &metadata) != nil || metadata.Mode != unix.S_IFDIR|0710 || metadata.Uid != 0 || metadata.Gid != gid {
			return ErrConflict
		}
		return ctx.Err()
	}
	return withGuestStorageImagePath(ctx, root, parent, image.SHA256, checkParent, func(file *os.File, checkPath func(context.Context) error) error {
		qualify := func() error {
			if err := checkPath(ctx); err != nil {
				return err
			}
			var metadata unix.Stat_t
			if unix.Fstat(int(file.Fd()), &metadata) != nil || metadata.Mode != unix.S_IFREG|0440 || metadata.Uid != 0 || metadata.Gid != gid || metadata.Nlink != 1 || metadata.Size != image.Bytes {
				return ErrConflict
			}
			for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
					return ErrConflict
				}
			}
			return checkPath(ctx)
		}
		if err := qualify(); err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(hash, contextReader{ctx, io.NewSectionReader(file, 0, image.Bytes+1)})
		if err != nil {
			return err
		}
		if n != image.Bytes || hex.EncodeToString(hash.Sum(nil)) != image.SHA256 {
			return catalog.ErrUntrusted
		}
		return qualify()
	})
}
