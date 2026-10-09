//go:build linux

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// prepareReservedDomain is entered only while the caller retains start and
// maintenance exclusion. It connects resource preparation to live host checks.
func (m *Manager) prepareReservedDomain(ctx context.Context, d Domain) error {
	return m.withPreparedReservedDomain(ctx, d, func(ctx context.Context, checkAccounts, checkPrepared func(context.Context) error) error {
		return checkPrepared(ctx)
	})
}

// Keep account authority retained while the caller consumes prepared storage.
// checkPrepared requires a stopped guest and is valid before activation only.
// checkAccounts remains valid during launch; the consumer must independently
// verify the running domain before publishing its runtime state.
func (m *Manager) withPreparedReservedDomain(ctx context.Context, d Domain, use func(context.Context, func(context.Context) error, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPolicy
	}
	if err := m.checkReservedDomainStopped(ctx, d); err != nil {
		return fmt.Errorf("qualify stopped reserved domain: %w", err)
	}
	host, err := os.OpenRoot("/")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, host.Close()) }()
	return withReservedAccountDirectory(ctx, host, func(ctx context.Context, checkAccounts func(context.Context) error) error {
		stopped := func(ctx context.Context) error {
			if err := checkAccounts(ctx); err != nil {
				return err
			}
			if err := m.checkReservedDomainStopped(ctx, d); err != nil {
				return err
			}
			return checkAccounts(ctx)
		}
		prepared, err := m.prepareReservedResources(ctx, d, stopped)
		if err != nil {
			return fmt.Errorf("prepare reserved storage and channel: %w", err)
		}
		checkPrepared := func(ctx context.Context) error {
			if err := m.qualifyReservedResources(ctx, d, prepared, stopped); err != nil {
				return fmt.Errorf("recheck prepared reserved resources: %w", err)
			}
			return nil
		}
		if err := checkPrepared(ctx); err != nil {
			return err
		}
		if err := use(ctx, checkAccounts, checkPrepared); err != nil {
			return err
		}
		return checkAccounts(ctx)
	})
}

// checkReservedDomainStopped supplies the live checks used while the manager
// retains its start and maintenance locks. Observations do not replace those
// locks or exclude unrelated privileged process and account writers.
func (m *Manager) checkReservedDomainStopped(ctx context.Context, d Domain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil || m.GuestUIDPool == nil || os.Geteuid() != 0 {
		return ErrPolicy
	}
	switch backend := m.Backend.(type) {
	case LinuxBackend:
	case *LinuxBackend:
		if backend == nil {
			return ErrPolicy
		}
	default:
		return ErrPolicy
	}
	if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
		return err
	}
	if _, err := m.channelAccessGID(); err != nil {
		return err
	}
	group, err := ObserveGuestKVMGroup(ctx)
	if err != nil {
		return err
	}
	if group != d.GuestGID {
		return ErrPolicy
	}
	if err := ObserveGuestUIDNameServiceEligibility(ctx); err != nil {
		return err
	}
	if err := ObserveGuestUIDAutomaticAllocation(ctx, *m.GuestUIDPool); err != nil {
		return err
	}
	accounts, err := ObserveLocalGuestUIDConflicts(ctx, *m.GuestUIDPool)
	if err != nil {
		return err
	}
	if accounts.Blocked[d.GuestUID] {
		return ErrPolicy
	}
	running, err := m.Backend.Running(ctx, d.ID)
	if err != nil {
		return err
	}
	if running {
		return ErrPolicy
	}
	processes, err := ObserveGuestUIDProcessConflicts(ctx, *m.GuestUIDPool)
	if err != nil {
		return err
	}
	if processes.Blocked[d.GuestUID] {
		return ErrPolicy
	}
	if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
		return err
	}
	return ctx.Err()
}
