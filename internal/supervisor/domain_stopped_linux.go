//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
)

// prepareReservedDomain is entered only while the caller retains start and
// maintenance exclusion. It connects resource preparation to live host checks.
func (m *Manager) prepareReservedDomain(ctx context.Context, d Domain) (result error) {
	if err := m.checkReservedDomainStopped(ctx, d); err != nil {
		return err
	}
	host, err := os.OpenRoot("/")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, host.Close()) }()
	return withReservedAccountDirectory(ctx, host, func(ctx context.Context, checkAccounts func(context.Context) error) error {
		_, err := m.prepareReservedResources(ctx, d, func(ctx context.Context) error {
			if err := checkAccounts(ctx); err != nil {
				return err
			}
			if err := m.checkReservedDomainStopped(ctx, d); err != nil {
				return err
			}
			return checkAccounts(ctx)
		})
		return err
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
