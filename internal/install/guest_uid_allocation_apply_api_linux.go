//go:build linux

package install

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// ApplyGuestUIDAllocationConfiguration applies the previously committed allocator change
// to an owned, empty installation behind its existing activation block. It
// retains that block and the original configuration after success. This does
// not reserve UID ranges, publish runtime policy or activate services.
func (e *Engine) ApplyGuestUIDAllocationConfiguration(ctx context.Context) (preview GuestUIDAllocationPreview, result error) {
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
	intent, err := e.loadGuestUIDAllocationIntent(ctx, owner)
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
			if err := supervisor.ObserveGuestUIDNameServiceEligibility(ctx); err != nil {
				return err
			}
			return checkRuntime(ctx)
		}
		return e.applyGuestUIDAllocationLocked(ctx, intent, guard)
	})
	if result != nil {
		return preview, result
	}
	return GuestUIDAllocationPreview{OwnerID: owner, First: intent.First, Last: intent.Last, Selection: intent.Selection, OriginalSHA256: intent.Proposal.OriginalSHA256, DesiredSHA256: intent.Proposal.DesiredSHA256, ChangesRequired: intent.Proposal.OriginalSHA256 != intent.Proposal.DesiredSHA256, IntentCommitted: true, ConfigurationApplied: true}, nil
}
