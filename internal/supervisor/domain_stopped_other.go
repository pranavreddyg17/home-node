//go:build !linux

package supervisor

import "context"

func (m *Manager) withPreparedReservedDomain(ctx context.Context, d Domain, use func(context.Context, func(context.Context) error, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}

func (m *Manager) prepareReservedDomain(ctx context.Context, d Domain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}

func (m *Manager) checkReservedDomainStopped(ctx context.Context, d Domain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
