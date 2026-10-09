package install

import "context"

// withGuestStorageExclusionLocked joins retained proposal and installation
// authority with the activation marker and live runtime guard. Caller holds
// e.mu and supplies exact destination admission for its journaled transition.
// This does not establish account-allocation exclusion or activate services.
func (e *Engine) withGuestStorageExclusionLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if observe == nil || destinations == nil || use == nil {
		return ErrPlan
	}
	_, err := e.withGuestStorageIntentGuarded(ctx, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkIntent func() error) error {
		return e.withRecoveryExclusionGuardedLocked(ctx, observe, destinations, func(ctx context.Context, checkRuntime func(context.Context) error) error {
			guard := func(ctx context.Context) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := checkIntent(); err != nil {
					return err
				}
				if err := checkRuntime(ctx); err != nil {
					return err
				}
				return checkIntent()
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
	return err
}
