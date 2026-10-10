//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/accountlock"
)

// Retain the shared descriptor lock under the installer's qualified directory.
func withGuestIdentityAllocationLock(ctx context.Context, directory *os.Root, use func(context.Context, func() error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return mapGuestIdentityAllocationLockError(accountlock.ErrInvalid)
	}
	err := accountlock.With(ctx, directory, func(ctx context.Context, check func() error) error {
		return use(ctx, func() error { return mapGuestIdentityAllocationLockError(check()) })
	})
	return mapGuestIdentityAllocationLockError(err)
}

func mapGuestIdentityAllocationLockError(err error) error {
	if errors.Is(err, accountlock.ErrInvalid) {
		return errors.Join(ErrPlan, err)
	}
	if errors.Is(err, accountlock.ErrConflict) {
		return errors.Join(ErrConflict, err)
	}
	return err
}
