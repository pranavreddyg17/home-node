//go:build !linux

package supervisor

import (
	"context"
	"os"
)

func openMaintenanceVolume(context.Context, string, string, int64) (*os.File, error) {
	return nil, ErrPolicy
}
