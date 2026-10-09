//go:build !linux

package supervisor

import "context"

func (m *Manager) verifyReservedDomainSocket(ctx context.Context, d Domain, revision int64) error {
	return m.checkReservedDomainSocket(ctx, d, revision, true)
}

func (m *Manager) checkReservedDomainSocket(ctx context.Context, d Domain, revision int64, allowCreate bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
