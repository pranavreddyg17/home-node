package supervisor

import (
	"context"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// A successful destroy request alone is not evidence that teardown completed.
func (m *Manager) stopDomain(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil || !guestproto.ValidID(id) {
		return ErrPolicy
	}
	if err := m.Backend.Stop(ctx, id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	running, err := m.Backend.Running(ctx, id)
	if err != nil || running {
		return errors.Join(ErrPolicy, err)
	}
	return ctx.Err()
}
