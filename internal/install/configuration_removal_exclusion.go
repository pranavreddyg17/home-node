package install

import (
	"context"
	"errors"
	"os"
	"reflect"
	"syscall"
)

// withConfigurationRemovalExclusionLocked retains the existing activation
// marker inode and installation journal across an ordered removal consumer.
// Caller holds e.mu. Observe must validate canonical removal progress, manager
// identity/dormancy, loaded activation conditions and guest emptiness. It is
// mandatory even for a ready retry; the marker is never recreated or released.
func (e *Engine) withConfigurationRemovalExclusionLocked(ctx context.Context, observe func(context.Context) error, use func(context.Context, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if observe == nil || use == nil {
		return ErrPlan
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" {
		return ErrConflict
	}
	if _, err := e.loadConfigurationRemovalIntent(installed); err != nil {
		return err
	}
	if err := e.requireRecoveryActivationBlock(ctx); err != nil {
		return err
	}
	marker, err := e.journalRoot.OpenFile("recovery-blocked", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, marker.Close()) }()
	before, err := marker.Stat()
	if err != nil {
		return err
	}
	retained := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := e.load()
		if err != nil || !reflect.DeepEqual(current, installed) {
			return ErrConflict
		}
		if _, err := e.loadConfigurationRemovalIntent(current); err != nil {
			return err
		}
		if err := e.requireRecoveryActivationBlock(ctx); err != nil {
			return err
		}
		after, err := e.journalRoot.Lstat("recovery-blocked")
		if err != nil || !os.SameFile(before, after) {
			return ErrConflict
		}
		return ctx.Err()
	}
	guard := func(ctx context.Context) error {
		if err := retained(ctx); err != nil {
			return err
		}
		if err := observe(ctx); err != nil {
			return err
		}
		return retained(ctx)
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := use(ctx, guard); err != nil {
		return err
	}
	return guard(ctx)
}

// removeConfigurationWithExclusionLocked composes retained authority and the
// journaled consumer. Observe remains the caller's native manager/guest gate.
func (e *Engine) removeConfigurationWithExclusionLocked(ctx context.Context, observe func(context.Context) error) error {
	return e.withConfigurationRemovalExclusionLocked(ctx, observe, func(ctx context.Context, guard func(context.Context) error) error {
		return e.removeConfigurationLocked(ctx, guard)
	})
}
