package supervisor

import "context"

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
