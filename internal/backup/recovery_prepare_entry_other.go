//go:build !linux

package backup

import (
	"context"
	"os"
)

// PreparePrivateRecovery requires Linux filesystem qualification and private
// SQLite rebinding; other hosts cannot prepare an installable recovery set.
func PreparePrivateRecovery(ctx context.Context, _ *os.Root, _ *os.Root, _ string, _ Manifest, _ RestorePolicy) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", ErrManifest
}
