package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// PrivateBackupManagement is the restricted controller bridge used by the
// backup process. It grants no access to the live management database.
type PrivateBackupManagement interface {
	ManagementSnapshots
	ManagementPublication
}

// RunPrivateBackup stages and publishes an already admitted, frozen job. The
// dispatcher must retain both maintenance barriers and exclusive repository
// and empty staging ownership. This operation does not acquire or release
// barriers, restore apps, or dispose staging; those lifecycle steps belong to
// the coordinator. A snapshot ID can accompany an acknowledgement error.
func RunPrivateBackup(ctx context.Context, management PrivateBackupManagement, disks MaintenanceDisks, repository RecoveryPublisher, device, managementToken, runtimeToken string, staging *os.Root, release string, catalogVersion int64, policy RestorePolicy) (snapshotID string, resultErr error) {
	if management == nil || disks == nil || repository == nil || staging == nil || !guestproto.ValidID(device) || !guestproto.ValidID(managementToken) || !guestproto.ValidID(runtimeToken) {
		return "", ErrManifest
	}
	manifest, err := StagePrivateRecoverySet(ctx, management, disks, device, managementToken, runtimeToken, staging, release, catalogVersion, policy)
	if err != nil {
		return "", err
	}
	return PublishPrivateRecoverySet(ctx, management, repository, device, managementToken, staging, manifest, policy)
}
