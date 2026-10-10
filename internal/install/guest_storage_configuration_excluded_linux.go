//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Caller holds e.mu. Consume already staged configuration under the existing
// activation block, runtime vacancy observers and shared account writer lock.
// This does not release activation, stage missing files or expose a CLI command.
func (e *Engine) publishGuestStorageConfigurationExcludedLocked(ctx context.Context, observe, destinations func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || observe == nil || destinations == nil || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrPlan
	}
	_, result = e.withGuestStorageIntentGuarded(ctx, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkPlan func() error) (result error) {
		current, err := e.load()
		if err != nil {
			return err
		}
		return e.withGuestStorageConfigurationIntent(ctx, current, plan, func(intent guestStorageConfigurationIntent, checkIntent func() error) (result error) {
			stage, err := e.loadGuestStorageConfigurationStage(ctx, current, plan)
			if err != nil {
				return err
			}
			record, err := json.Marshal(stage)
			if err != nil {
				return err
			}
			return e.withGuestIdentityRecordGuarded(ctx, "guest-storage-configuration-stage.json", record, func(ctx context.Context, checkStage func() error) (result error) {
				directory, err := e.host.Open("etc/homenode")
				if err != nil {
					return err
				}
				defer func() { result = errors.Join(result, directory.Close()) }()
				admit := func(ctx context.Context, current journal) error {
					for _, check := range []func() error{checkPlan, checkIntent, checkStage} {
						if err := check(); err != nil {
							return err
						}
					}
					return e.admitGuestStorageConfigurationInstallation(ctx, current, intent, stage, directory)
				}
				return e.withRecoveryInstallationExclusionGuardedLocked(ctx, observe, destinations, admit, func(ctx context.Context, checkRuntime func(context.Context) error) error {
					return e.withGuestStorageAccountExclusionLocked(ctx, checkRuntime, func(ctx context.Context, checkAccounts func(context.Context) error) error {
						guard := func(ctx context.Context) error {
							if err := checkAccounts(ctx); err != nil {
								return err
							}
							live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: plan.Identity.First, Last: plan.Identity.Last})
							if err != nil {
								return err
							}
							if !reflect.DeepEqual(live, plan) {
								return ErrConflict
							}
							accounts, err := e.loadAccountJournal()
							if err != nil {
								return err
							}
							if !accounts.Ready || accounts.OwnerID != plan.Identity.OwnerID {
								return ErrConflict
							}
							if err := supervisor.ObserveReservedStorageParents(ctx, "/var/lib/homenode/images", "/var/lib/homenode/volumes", "/run/homenode/guests", plan.GuestGID, uint32(accounts.Accounts.TransferGID)); err != nil {
								return err
							}
							return checkAccounts(ctx)
						}
						return e.publishGuestStorageConfigurationLocked(ctx, directory, plan, guard)
					})
				})
			})
		})
	})
	return result
}
