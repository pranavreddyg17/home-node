//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/accountlock"
	"golang.org/x/sys/unix"
)

// Caller retains launch and maintenance exclusion. The shared account writer
// protocol does not exclude arbitrary privileged account or process writers.
func withReservedAccountDirectory(ctx context.Context, host *os.Root, use func(context.Context, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if host == nil || use == nil || os.Geteuid() != 0 {
		return ErrPolicy
	}
	directory, err := host.OpenRoot("etc")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	parent, err := directory.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	var original unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &original) != nil || original.Mode&unix.S_IFMT != unix.S_IFDIR || original.Uid != 0 || original.Gid != 0 || original.Mode&0022 != 0 {
		return ErrPolicy
	}
	checkParent := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current unix.Stat_t
		opened, err := parent.Stat()
		named, pathErr := host.Lstat("etc")
		if err != nil || pathErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
			return ErrPolicy
		}
		return nil
	}
	if err := checkParent(ctx); err != nil {
		return err
	}
	err = accountlock.With(ctx, directory, func(ctx context.Context, checkLock func() error) error {
		guard := func(ctx context.Context) error {
			if err := checkParent(ctx); err != nil {
				return err
			}
			if err := checkLock(); err != nil {
				return err
			}
			return checkParent(ctx)
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := use(ctx, guard); err != nil {
			return err
		}
		return guard(ctx)
	})
	if errors.Is(err, accountlock.ErrInvalid) || errors.Is(err, accountlock.ErrConflict) {
		return errors.Join(ErrPolicy, err)
	}
	return err
}
