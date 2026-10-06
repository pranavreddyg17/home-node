//go:build linux

package supervisor

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
	"time"
)

func verifyGuestDACProcess(ctx context.Context, pid int, uid, gid uint32) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || pid < 1 {
		return ErrPolicy
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	var fs unix.Statfs_t
	if unix.Fstatfs(int(directory.Fd()), &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return ErrPolicy
	}
	if err = qualifyGuestUIDNamespace(root); err != nil {
		return err
	}
	count, verified := 0, 0
	err = withGuestUIDTaskStatus(ctx, root, strconv.Itoa(pid), &count, func(status []byte) error {
		if err := validateGuestDACCredentials(status, uid, gid); err != nil {
			return err
		}
		verified++
		return nil
	})
	if err != nil {
		return err
	}
	if verified == 0 {
		return ErrPolicy
	}
	return ctx.Err()
}
