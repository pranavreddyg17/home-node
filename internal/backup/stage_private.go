package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// ManagementSnapshots is the restricted private controller bridge. Backup
// staging receives sanitized recovery data without opening management storage.
type ManagementSnapshots interface {
	StageManagementSnapshot(context.Context, string, string, *os.Root) error
	ConfirmStaging(context.Context, string, string) error
}

// StagePrivateRecoverySet qualifies pinned disks and obtains management state
// through the authenticated controller bridge. Both owned maintenance barriers
// and exclusive staging ownership must remain held for the entire operation.
func StagePrivateRecoverySet(ctx context.Context, management ManagementSnapshots, disks MaintenanceDisks, device, managementToken, runtimeToken string, root *os.Root, release string, catalogVersion int64, policy RestorePolicy) (Manifest, error) {
	if management == nil || disks == nil || !guestproto.ValidID(device) {
		return Manifest{}, ErrManifest
	}
	return stageRecoverySet(ctx, QualifiedMaintenanceDisks{Source: disks}, managementToken, runtimeToken, root, release, catalogVersion, policy,
		func(ctx context.Context, root *os.Root) error {
			return management.StageManagementSnapshot(ctx, managementToken, device, root)
		},
		func(ctx context.Context) error { return management.ConfirmStaging(ctx, managementToken, device) })
}
