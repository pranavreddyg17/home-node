package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// RecoveryInstallDisk describes a qualified disconnected data disk with a new
// target identity. It grants no UID lease, runtime or restored device authority.
type RecoveryInstallDisk struct {
	Workload     string
	SourceName   string
	Bytes        int64
	SourceSHA256 string
	ImageSHA256  string
	InstanceID   string
}

// RecoveryInstallInventory requires exclusive ownership of recovered staging
// throughout qualification and later installation. The returned inventory is
// transient; installation must journal it before effects and retain/revalidate
// staging rather than treating this value as durable authority.
func RecoveryInstallInventory(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) ([]RecoveryInstallDisk, error) {
	if err := QualifyRecoveryDisks(ctx, root, manifest, policy); err != nil {
		return nil, err
	}
	disks := make([]RecoveryInstallDisk, 0, len(manifest.Files)-1)
	for _, file := range manifest.Files {
		if file.Workload == "management" {
			continue
		}
		disks = append(disks, RecoveryInstallDisk{Workload: file.Workload, SourceName: file.Name, Bytes: file.Bytes, SourceSHA256: file.SHA256, ImageSHA256: policy.ApprovedImages[file.Workload], InstanceID: state.Random()})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return disks, nil
}
