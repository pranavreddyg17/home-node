//go:build !linux && !darwin

package backup

import (
	"context"
	"os"
)

func lockMaintenanceRunner(context.Context, string) (*os.File, error) {
	return nil, ErrMaintenanceRunner
}
