//go:build !linux

package backup

import (
	"context"
	"os"
)

func RestoreAndPreparePrivateRecovery(ctx context.Context, _ *Repository, _ *JobStaging, _ *os.Root, _ string, _ RestorePolicy) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", ErrManifest
}
