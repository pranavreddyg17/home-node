//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ObserveReservedStorageParents checks installed layout without changing it.
// This is a startup observation; launch retains its own runtime/path authority.
func ObserveReservedStorageParents(ctx context.Context, images, volumes, channels string, guestGID, accessGID uint32) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || guestGID == 0 || accessGID == 0 || guestGID == accessGID || guestGID > 1<<31-1 || accessGID > 1<<31-1 {
		return ErrPolicy
	}
	paths := []string{images, volumes, channels}
	seen := map[string]bool{}
	files := make([]*os.File, 0, 3)
	stats := make([]unix.Stat_t, 0, 3)
	defer func() {
		for _, file := range files {
			result = errors.Join(result, file.Close())
		}
	}()
	for i, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || seen[path] {
			return ErrPolicy
		}
		seen[path] = true
		file, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		files = append(files, file)
		var stat unix.Stat_t
		gid := guestGID
		if i == 2 {
			gid = accessGID
		}
		if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode != unix.S_IFDIR|0710 || stat.Uid != 0 || stat.Gid != gid {
			return ErrPolicy
		}
		stats = append(stats, stat)
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for i, file := range files {
			var current, named unix.Stat_t
			before := stats[i]
			if unix.Fstat(int(file.Fd()), &current) != nil || unix.Lstat(paths[i], &named) != nil || current.Dev != before.Dev || current.Ino != before.Ino || current.Mode != before.Mode || current.Uid != before.Uid || current.Gid != before.Gid || named.Dev != current.Dev || named.Ino != current.Ino || named.Mode != current.Mode || named.Uid != current.Uid || named.Gid != current.Gid || !samePathMount(int(file.Fd()), unix.AT_FDCWD, paths[i]) {
				return ErrPolicy
			}
			for _, acl := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(file.Fd()), acl, nil); !errors.Is(err, unix.ENODATA) {
					return errors.Join(ErrPolicy, err)
				}
			}
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	return check()
}
