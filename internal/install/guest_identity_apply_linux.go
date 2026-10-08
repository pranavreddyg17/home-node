//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Caller holds e.mu, owned-account authority and a retained activation barrier.
// This transaction also retains the shared account-writer allocation lock.
// Intent must already be durably committed from
// independently observed host bytes. Failures preserve records and both files.
func (e *Engine) applyGuestIdentityNameServicesLocked(ctx context.Context, intent guestIdentityNameServiceIntent, guard func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || os.Geteuid() != 0 {
		return ErrPlan
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return e.withGuestIdentityNameServiceIntentGuarded(ctx, intent, func(ctx context.Context, checkIntent func() error) (result error) {
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
		retainedGuard := func(ctx context.Context) error {
			if err := checkIntent(); err != nil {
				return err
			}
			if err := guard(ctx); err != nil {
				return err
			}
			if err := checkIntent(); err != nil {
				return err
			}
			var current unix.Stat_t
			opened, statErr := parent.Stat()
			named, nameErr := e.host.Lstat("etc")
			if unix.Fstat(int(parent.Fd()), &current) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Mode != original.Mode || current.Uid != 0 || current.Gid != 0 || statErr != nil || nameErr != nil || !os.SameFile(opened, named) {
				return ErrConflict
			}
			return ctx.Err()
		}
		if err := retainedGuard(ctx); err != nil {
			return err
		}
		return withGuestIdentityAllocationLock(ctx, directory, func(ctx context.Context, checkAllocation func() error) error {
			lockedGuard := func(ctx context.Context) error {
				if err := checkAllocation(); err != nil {
					return err
				}
				if err := retainedGuard(ctx); err != nil {
					return err
				}
				return checkAllocation()
			}
			if err := lockedGuard(ctx); err != nil {
				return err
			}
			stage, err := e.loadGuestIdentityNameServiceStage(ctx, intent)
			if errors.Is(err, os.ErrNotExist) {
				err = e.withGuestIdentityHostSource(ctx, intent, func(ctx context.Context, source *os.File) error {
					var stageErr error
					stage, stageErr = e.stageGuestIdentityNameServices(ctx, directory, source, intent, lockedGuard)
					return stageErr
				})
			}
			if err != nil {
				return err
			}
			return e.publishGuestIdentityNameServices(ctx, parent, stage, intent, lockedGuard)
		})
	})
}
