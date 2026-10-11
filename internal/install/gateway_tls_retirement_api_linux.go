//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"time"
)

// RetireGatewayTLS makes retained TLS material root-only before gateway numeric
// identities may be released. It retains TLS contents, application data, backup
// data and the existing activation block; no identity is removed here.
func (e *Engine) RetireGatewayTLS(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	installed, err := e.load()
	if err != nil || !installedGateway(installed) {
		return fmt.Errorf("TLS retirement installation: %w", ErrConflict)
	}
	base, err := e.loadAccountJournal()
	if err != nil || !base.Ready {
		return fmt.Errorf("TLS retirement accounts: %w", ErrConflict)
	}
	identity, err := e.observeGatewayAccountWith(ctx, base, nativeGatewayProvisioner{})
	if err != nil || identity == nil {
		return fmt.Errorf("TLS retirement identity: %w", ErrConflict)
	}
	data, err := e.readGatewayTLSRetirementIntent()
	prepared := err == nil
	var a *gatewayTLSAdmission
	immutable := installed
	if prepared {
		var candidate gatewayTLSRetirementIntent
		if json.Unmarshal(data, &candidate) != nil || candidate.Plan.Identity != *identity {
			return ErrConflict
		}
		if _, err := decodeGatewayTLSRetirementIntent(data, candidate.Plan); err != nil {
			return err
		}
		immutable, err = gatewayTLSInstallationSource(installed, candidate)
		if err != nil {
			return err
		}
		a, err = e.reopenGatewayTLSRetirementLocked(ctx, candidate.Plan)
	} else {
		if !os.IsNotExist(err) {
			return err
		}
		a, err = e.admitGatewayTLSRetirementLocked(ctx, *identity)
	}
	if err != nil {
		return fmt.Errorf("TLS retirement artifacts: %w", err)
	}
	defer func() { result = errors.Join(result, a.Close()) }()
	admit := func(ctx context.Context, current journal) error {
		if prepared {
			return e.admitGatewayTLSInstallationLocked(ctx, current, immutable, a)
		}
		if !reflect.DeepEqual(current, immutable) {
			return ErrConflict
		}
		for _, item := range immutable.Items {
			if err := e.matches(item); err != nil {
				return err
			}
		}
		return e.verifyGatewayTLSAdmissionLocked(ctx, a)
	}
	quiesced := false
	observe := func(ctx context.Context) error {
		live, err := e.observeGatewayAccountWith(ctx, base, nativeGatewayProvisioner{})
		if err != nil || live == nil || *live != *identity {
			return ErrConflict
		}
		if !quiesced {
			if err := quiesceRecoveryManagerWith(ctx, exec.CommandContext, e.requireRecoveryActivationBlock, ObserveRecoveryGuestsEmpty, true); err != nil {
				return err
			}
			quiesced = true
		}
		if err := ObserveRecoveryActivationConditions(ctx, true); err != nil {
			return err
		}
		if err := ObserveRecoveryServicesDormant(ctx, true); err != nil {
			return err
		}
		return ObserveRecoveryGuestsEmpty(ctx)
	}
	result = e.withRecoveryInstallationExclusionGuardedLocked(ctx, observe, func(ctx context.Context) error { return ctx.Err() }, admit, func(ctx context.Context, retained func(context.Context) error) error {
		guard := func() error { return retained(ctx) }
		if !prepared {
			intent := gatewayTLSRetirementIntent{Version: 1, Plan: a.Plan, Sources: a.Sources}
			if err := e.prepareGatewayTLSRetirementIntentLocked(ctx, intent, guard); err != nil {
				return fmt.Errorf("TLS retirement intent: %w", err)
			}
			prepared = true
		}
		if err := e.applyGatewayTLSRetirementLocked(ctx, a, guard); err != nil {
			return fmt.Errorf("TLS retirement permissions: %w", err)
		}
		if err := e.publishGatewayTLSRetirementLocked(ctx, a, guard); err != nil {
			return fmt.Errorf("TLS retirement publication: %w", err)
		}
		return nil
	})
	if result != nil {
		return fmt.Errorf("TLS retirement exclusion: %w", result)
	}
	return nil
}
