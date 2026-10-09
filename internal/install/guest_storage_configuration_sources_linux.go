//go:build linux

package install

import (
	"context"
	"os"
	"reflect"
)

// Retain both original host files and the independently matched immutable
// transition while preparing replacement files. The supplied guard must retain
// installation, storage, account and dormant-runtime authority. This scope does
// not admit partially published configurations or authorize service activation.
func (e *Engine) withGuestStorageConfigurationSources(ctx context.Context, current journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error, use func(context.Context, guestStorageConfigurationIntent, *os.File, *os.File, func() error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard == nil || use == nil {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	return e.withGuestStorageConfigurationIntent(ctx, current, plan, func(intent guestStorageConfigurationIntent, checkIntent func() error) error {
		if !reflect.DeepEqual(current, intent.Original) {
			return ErrConflict
		}
		return e.withGuestStorageConfigurationSource(ctx, "runtime-policy.json", string(intent.SourcePolicy), func(ctx context.Context, policy *os.File, checkPolicy func() error) error {
			return e.withGuestStorageConfigurationSource(ctx, "services.env", string(intent.SourceEnvironment), func(ctx context.Context, environment *os.File, checkEnvironment func() error) error {
				check := func() error {
					if err := guard(ctx); err != nil {
						return err
					}
					for _, verify := range []func() error{checkIntent, checkPolicy, checkEnvironment} {
						if err := verify(); err != nil {
							return err
						}
					}
					return guard(ctx)
				}
				if err := check(); err != nil {
					return err
				}
				if err := use(ctx, intent, policy, environment, check); err != nil {
					return err
				}
				return check()
			})
		})
	})
}
