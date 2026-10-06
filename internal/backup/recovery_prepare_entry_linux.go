//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
)

// PreparePrivateRecovery prepares verified disconnected data and management
// files under exclusive maintenance leases on both private runner-owned roots.
// Source is an already restored selected snapshot; policy must come from the
// replacement host's trusted catalog. This does not install ownership/policy,
// enroll clients or activate a live controller. Returned names stay in staging.
func PreparePrivateRecovery(ctx context.Context, source, destination *os.Root, snapshotID string, manifest Manifest, policy RestorePolicy) (name string, result error) {
	sourceLock, destinationLock, err := lockRecoveryRoots(ctx, source, destination)
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, destinationLock.Close(), sourceLock.Close())
		if result != nil {
			name = ""
		}
	}()
	return prepareRecoveryManagement(ctx, source, destination, snapshotID, manifest, policy)
}
