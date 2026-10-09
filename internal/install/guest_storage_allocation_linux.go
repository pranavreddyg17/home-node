//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Caller holds e.mu. The shared account-writer protocol is retained alongside
// the intent, installation and activation guards; arbitrary root writers remain
// outside this exclusion. No successful return enables services.
func (e *Engine) withGuestStorageAllocationExclusionLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	return e.withGuestStorageExclusionLocked(ctx, observe, destinations, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkRuntime func(context.Context) error) (result error) {
		directory, err := e.host.OpenRoot("etc")
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
			return ErrConflict
		}
		checkParent := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			var current unix.Stat_t
			opened, err := parent.Stat()
			named, pathErr := e.host.Lstat("etc")
			if err != nil || pathErr != nil || !os.SameFile(opened, named) || unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != original.Uid || current.Gid != original.Gid {
				return ErrConflict
			}
			return nil
		}
		if err := checkParent(); err != nil {
			return err
		}
		return withGuestIdentityAllocationLock(ctx, directory, func(ctx context.Context, checkAllocation func() error) error {
			guard := func(ctx context.Context) error {
				if err := checkParent(); err != nil {
					return err
				}
				if err := checkAllocation(); err != nil {
					return err
				}
				if err := checkRuntime(ctx); err != nil {
					return err
				}
				if err := checkAllocation(); err != nil {
					return err
				}
				return checkParent()
			}
			if err := guard(ctx); err != nil {
				return err
			}
			if err := use(ctx, plan, guard); err != nil {
				return err
			}
			return guard(ctx)
		})
	})
}
