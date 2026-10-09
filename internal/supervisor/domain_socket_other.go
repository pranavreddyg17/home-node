//go:build !linux

package supervisor

import "context"

func (m *Manager) verifyReservedDomainSocket(ctx context.Context, d Domain, revision int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
