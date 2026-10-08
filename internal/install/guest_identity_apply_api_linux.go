//go:build linux

package install

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// ApplyGuestIdentityConfiguration applies the previously committed NSS change
// to an owned, empty installation behind its existing activation block. It
// retains that block and the original configuration after success. This does
// not reserve UID ranges, publish runtime policy or activate services.
func (e *Engine) ApplyGuestIdentityConfiguration(ctx context.Context) (preview GuestIdentityConfigurationPreview, result error) {
	if err := ctx.Err(); err != nil {
		return preview, err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return preview, ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !e.mu.TryLock() {
		return preview, ErrConflict
	}
	defer e.mu.Unlock()
	owner, err := e.inspectGuestIdentityAccountsLocked(ctx)
	if err != nil {
		return preview, err
	}
	intent, err := e.loadGuestIdentityNameServiceIntent(ctx, owner)
	if err != nil {
		return preview, err
	}
	quiesced := false
	observe := func(ctx context.Context) error {
		if !quiesced {
			if err := quiesceRecoveryManagerWith(ctx, exec.CommandContext, e.requireRecoveryActivationBlock, ObserveRecoveryGuestsEmpty); err != nil {
				return err
			}
			quiesced = true
			return nil
		}
		if err := ObserveRecoveryActivationConditions(ctx); err != nil {
			return err
		}
		if err := ObserveRecoveryServicesDormant(ctx); err != nil {
			return err
		}
		return ObserveRecoveryGuestsEmpty(ctx)
	}
	result = e.withRecoveryExclusionGuardedLocked(ctx, observe, e.observeRecoveryDestinationVacancy, func(ctx context.Context, checkRuntime func(context.Context) error) error {
		guard := func(ctx context.Context) error {
			if err := checkRuntime(ctx); err != nil {
				return err
			}
			currentOwner, err := e.inspectGuestIdentityAccountsLocked(ctx)
			if err != nil {
				return err
			}
			if currentOwner != owner {
				return ErrConflict
			}
			return checkRuntime(ctx)
		}
		return e.applyGuestIdentityNameServicesLocked(ctx, intent, guard)
	})
	if result != nil {
		return preview, result
	}
	return GuestIdentityConfigurationPreview{OwnerID: owner, OriginalSHA256: intent.Proposal.OriginalSHA256, DesiredSHA256: intent.Proposal.DesiredSHA256, ChangesRequired: intent.Proposal.OriginalSHA256 != intent.Proposal.DesiredSHA256, IntentCommitted: true, ConfigurationApplied: true}, nil
}
