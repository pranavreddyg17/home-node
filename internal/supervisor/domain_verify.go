package supervisor

import (
	"context"
	"strconv"
)

// verifyPreparedDomain must finish before the manager publishes running state.
func (m *Manager) verifyPreparedDomain(ctx context.Context, d Domain, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil {
		return ErrPolicy
	}
	if d.GuestUID != 0 {
		switch m.Backend.(type) {
		case LinuxBackend, *LinuxBackend:
			return m.verifyReservedDomainSocket(ctx, d, revision)
		}
	}
	return m.Backend.Verify(ctx, d)
}

// verifyRunningDomain cannot establish a missing socket receipt. Runtime audits
// authenticate existing provenance before entering live backend verification.
func (m *Manager) verifyRunningDomain(ctx context.Context, d Domain, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil {
		return ErrPolicy
	}
	if d.GuestUID != 0 {
		switch m.Backend.(type) {
		case LinuxBackend, *LinuxBackend:
			if _, err := m.loadActiveChannelSocketIntent(ctx, d, revision); err != nil {
				return err
			}
			return m.checkReservedDomainSocket(ctx, d, revision, false)
		}
	}
	return m.Backend.Verify(ctx, d)
}

func (m *Manager) verifyShuttingDownDomain(ctx context.Context, d Domain, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil {
		return ErrPolicy
	}
	if d.GuestUID != 0 {
		switch m.Backend.(type) {
		case LinuxBackend, *LinuxBackend:
			if m.Store == nil {
				return ErrPolicy
			}
			var saved string
			if err := m.Store.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, shutdownSocketRevisionKey(d.ID)).Scan(&saved); err != nil {
				return err
			}
			launchRevision, err := strconv.ParseInt(saved, 10, 64)
			if err != nil || launchRevision < 1 || launchRevision >= revision {
				return ErrPolicy
			}
			return m.verifyRunningDomain(ctx, d, launchRevision)
		}
	}
	return m.Backend.Verify(ctx, d)
}
