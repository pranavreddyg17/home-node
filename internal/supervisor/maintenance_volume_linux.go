//go:build linux

package supervisor

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"golang.org/x/sys/unix"
)

// Pin the admitted inode read-only; never follow guest/controller supplied
// paths, mount the guest filesystem, or invoke its filesystem parser as root.
func openMaintenanceVolume(ctx context.Context, directory, id string, size int64) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if os.Geteuid() != 0 || !guestproto.ValidID(id) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || size < 16<<20 || size > 512<<30 {
		return nil, ErrPolicy
	}
	before, err := os.Lstat(directory)
	if err != nil || !before.IsDir() || before.Mode().Perm()&0022 != 0 {
		return nil, ErrPolicy
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer parent.Close()
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
		file.Close()
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
