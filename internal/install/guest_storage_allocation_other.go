//go:build !linux

package install

import "context"

func (e *Engine) withGuestStorageAllocationExclusionLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPlan
}
