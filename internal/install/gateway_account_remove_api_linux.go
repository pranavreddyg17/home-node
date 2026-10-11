//go:build linux

package install

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// RemoveGatewayAccount stops the owned installation behind an existing activation
// block and removes only its journaled gateway identity. Configuration, workload
// data, backups and the activation block remain retained. This is one uninstall
// phase, not a completed installation removal or an activation-ready state.
func (e *Engine) RemoveGatewayAccount(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	installed, err := e.load()
	if err != nil || !installedGateway(installed) {
		return ErrConflict
	}
	base, err := e.loadAccountJournal()
	if err != nil {
		return err
	}
	intent, err := e.loadGatewayRemovalIntent(base)
	if err != nil {
		return err
	}
	// Retained TLS directories or service data must first undergo a separately
	// journaled ownership transition. Do not free a referenced numeric identity.
	if len(gatewayRemovalIdentityReferences(installed, intent.Plan.Identity)) != 0 {
		return ErrConflict
	}
	quiesced := false
	observe := func(ctx context.Context) error {
		if !quiesced {
			if err := quiesceRecoveryManagerWith(ctx, exec.CommandContext, e.requireRecoveryActivationBlock, ObserveRecoveryGuestsEmpty, true); err != nil {
				return err
			}
			quiesced = true
			return nil
		}
		if err := ObserveRecoveryActivationConditions(ctx, true); err != nil {
			return err
		}
		if err := ObserveRecoveryServicesDormant(ctx, true); err != nil {
			return err
		}
		return ObserveRecoveryGuestsEmpty(ctx)
	}
	// Teardown retains existing app databases and backup repositories. Recovery's
	// vacant-destination predicate would incorrectly reject their retained data.
	retainedData := func(ctx context.Context) error { return ctx.Err() }
	return e.withRecoveryExclusionGuardedLocked(ctx, observe, retainedData, func(ctx context.Context, guard func(context.Context) error) error {
		return e.removeGatewayAccountLocked(ctx, nativeGatewayProvisioner{}, guard)
	})
}
