//go:build !linux

package supervisor

import (
	"context"
	"os"
)

func (m *Manager) withReservedMaintenanceDisk(ctx context.Context, token string, instance Instance, copyDisk func(context.Context, *os.File, Instance) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrPolicy
}
