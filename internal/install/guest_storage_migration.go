package install

import (
	"context"
	"reflect"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Recovery joins the original image provenance to the current qualified
// storage plan. The consumer must retain each image descriptor and pathname.
func (e *Engine) withQualifiedGuestStorageImagesMigrationLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, guestStorageImagesIntent, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	return e.withQualifiedGuestStorageMigrationLocked(ctx, observe, destinations, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkMigration func(context.Context) error) error {
		return e.withGuestStorageImagesIntentGuarded(ctx, func(ctx context.Context, intent guestStorageImagesIntent, checkImages func() error) error {
			if !reflect.DeepEqual(plan, intent.Plan) {
				return ErrConflict
			}
			guard := func(ctx context.Context) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := checkImages(); err != nil {
					return err
				}
				if err := checkMigration(ctx); err != nil {
					return err
				}
				return checkImages()
			}
			if err := guard(ctx); err != nil {
				return err
			}
			if err := use(ctx, intent, guard); err != nil {
				return err
			}
			return guard(ctx)
		})
	})
}

// Caller holds e.mu and provides exact journaled destination admission. Live
// account/device observations remain within the retained exclusion scope.
// A consumer still must pin, journal and recheck every storage inode it changes.
func (e *Engine) withQualifiedGuestStorageMigrationLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	return e.withGuestStorageAllocationExclusionLocked(ctx, observe, destinations, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkExclusion func(context.Context) error) error {
		guard := func(ctx context.Context) error {
			if err := checkExclusion(ctx); err != nil {
				return err
			}
			live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: plan.Identity.First, Last: plan.Identity.Last})
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(live, plan) {
				return ErrConflict
			}
			return checkExclusion(ctx)
		}
		if err := guard(ctx); err != nil {
			return err
		}
		if err := use(ctx, plan, guard); err != nil {
			return err
		}
		return guard(ctx)
	})
}
