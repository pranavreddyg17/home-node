//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"reflect"
)

// The caller already holds shared runtime admission (as Apply does). Retain
// start serialization across the whole callback, including descriptor closure.
// Process observations supplement backend absence; they are not proof against
// an escaped process racing fork, nor authority for releasing a reserved UID.
func (m *Manager) withVideoRetirementStartExclusion(ctx context.Context, r Request, use func(videoRetirementSnapshot, func(context.Context) error) error) error {
	if m == nil || m.Backend == nil {
		return ErrPolicy
	}
	return m.withVideoRetirementStartExclusionObserved(ctx, r, m.Backend.Running, use)
}

// Only internal tests substitute backend absence. Procfs observation remains real.
func (m *Manager) withVideoRetirementStartExclusionObserved(ctx context.Context, r Request, runningNow func(context.Context, string) (bool, error), use func(videoRetirementSnapshot, func(context.Context) error) error) error {
	if m == nil || m.Store == nil || m.Backend == nil || m.GuestUIDPool == nil || os.Geteuid() != 0 || use == nil || runningNow == nil {
		return ErrPolicy
	}
	unlock, err := m.lockStart(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if m.shuttingDown {
		return ErrPolicy
	}
	snapshot, err := m.loadVideoRetirementSnapshot(ctx, r)
	if err != nil {
		return err
	}
	check := func(scope context.Context) error {
		if err := scope.Err(); err != nil {
			return err
		}
		m.stopMu.Lock()
		stopping := m.activeStops[r.InstanceID] != 0
		m.stopMu.Unlock()
		if stopping {
			return ErrPolicy
		}
		running, err := runningNow(scope, r.InstanceID)
		if err != nil {
			return err
		}
		if running {
			return ErrPolicy
		}
		pool, err := ObserveGuestUIDProcessConflicts(scope, *m.GuestUIDPool)
		if err != nil {
			return err
		}
		if pool.Blocked[snapshot.Channel.UID] {
			return ErrPolicy
		}
		current, err := m.loadVideoRetirementSnapshot(scope, r)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, snapshot) {
			return ErrPolicy
		}
		return scope.Err()
	}
	if err := check(ctx); err != nil {
		return err
	}
	if err := use(snapshot, check); err != nil {
		return err
	}
	return errors.Join(check(ctx), ctx.Err())
}
