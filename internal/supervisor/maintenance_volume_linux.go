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

// Pin the admitted inode read-only; never follow guest/controller supplied
// paths, mount the guest filesystem, or invoke its filesystem parser as root.
func openMaintenanceVolume(ctx context.Context, directory, id string, size int64) (result *os.File, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if os.Geteuid() != 0 || !guestproto.ValidID(id) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || size < 16<<20 || size > 512<<30 {
		return nil, ErrPolicy
	}
	before, err := os.Lstat(directory)
	if err != nil || !before.IsDir() || before.Mode().Perm()&0022 != 0 || before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
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
	var native unix.Stat_t
	if err != nil || !os.SameFile(before, opened) || unix.Fstat(int(parent.Fd()), &native) != nil || native.Uid != 0 {
		return nil, ErrPolicy
	}
	file, err := root.OpenFile(id+".raw", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if err = admitVolume(file, size); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	pinned, err := file.Stat()
	current, pathErr := root.Lstat(id + ".raw")
	parentCurrent, parentErr := os.Lstat(directory)
	var finalParent unix.Stat_t
	if err != nil || pathErr != nil || parentErr != nil || !os.SameFile(pinned, current) || !os.SameFile(before, parentCurrent) || parentCurrent.Mode() != before.Mode() || unix.Fstat(int(parent.Fd()), &finalParent) != nil || finalParent.Dev != native.Dev || finalParent.Ino != native.Ino || finalParent.Uid != native.Uid || finalParent.Gid != native.Gid || finalParent.Mode != native.Mode {
		return nil, errors.Join(ErrPolicy, file.Close())
	}
	// Re-admit the retained disk after the pathname/parent reads; those reads
	// do not freeze inode metadata or authorize a changed owner/link/mode.
	if err := admitVolume(file, size); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
