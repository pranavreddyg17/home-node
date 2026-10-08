//go:build linux

package install

import (
	"context"
	"os"
)

// The consumer stages only; it must leave the source pathname and bytes intact.
func (e *Engine) withGuestUIDAllocatorHostSource(ctx context.Context, intent guestUIDAllocationIntent, use func(context.Context, *os.File, func() error) error) error {
	if use == nil {
		return ErrPlan
	}
	return e.withGuestUIDAllocationIntent(ctx, intent, func(ctx context.Context, checkIntent func() error) error {
		return e.withGuestIdentityConfigurationSource(ctx, "login.defs", intent.Original, func(ctx context.Context, file *os.File, checkSource func() error) error {
			guard := func() error {
				if err := checkSource(); err != nil {
					return err
				}
				if err := checkIntent(); err != nil {
					return err
				}
				return checkSource()
			}
			if err := guard(); err != nil {
				return err
			}
			if err := use(ctx, file, guard); err != nil {
				return err
			}
			return guard()
		})
	})
}
