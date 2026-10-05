package backup

import (
	"context"
	"errors"
	"os"
)

// prepareRecoveryDisks prepares disconnected disk files in an exclusively owned
// private destination. The selected snapshot must have been restored into an
// exclusively owned source under current trusted policy. The immutable journal
// precedes effects; retries retain identities and requalify sources. Incomplete
// or conflicting occupied stages are refused for explicit repair. No management
// import, UID lease, policy activation or runtime start occurs here.
func prepareRecoveryDisks(ctx context.Context, source, destination *os.Root, snapshotID string, manifest Manifest, policy RestorePolicy) ([]RecoveryInstallDisk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source == nil || destination == nil || !repositoryPattern.MatchString(snapshotID) {
		return nil, ErrManifest
	}
	plan, err := loadRecoveryInstallPlan(ctx, destination)
	if errors.Is(err, os.ErrNotExist) {
		inventory, qualificationErr := RecoveryInstallInventory(ctx, source, manifest, policy)
		if qualificationErr != nil {
			return nil, qualificationErr
		}
		plan = recoveryInstallPlan{Version: 1, SnapshotID: snapshotID, Disks: inventory}
		if err = createRecoveryInstallPlan(ctx, destination, plan); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if plan.SnapshotID != snapshotID {
		return nil, ErrManifest
	}
	if err = requalifyRecoveryInstallPlan(ctx, source, plan, manifest, policy); err != nil {
		return nil, err
	}
	for _, disk := range plan.Disks {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		stage := ".recovery-" + disk.InstanceID + ".stage"
		if _, statErr := destination.Lstat(stage); errors.Is(statErr, os.ErrNotExist) {
			// Missing stage cannot establish ownership of an occupied final name.
			if _, finalErr := destination.Lstat(disk.InstanceID + ".raw"); !errors.Is(finalErr, os.ErrNotExist) {
				return nil, ErrManifest
			}
			if err = copyRecoveryDisk(ctx, source, destination, disk, stage); err != nil {
				return nil, err
			}
		} else if statErr != nil {
			return nil, statErr
		}
		if _, err = publishRecoveryDisk(ctx, destination, disk); err != nil {
			return nil, err
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return append(make([]RecoveryInstallDisk, 0, len(plan.Disks)), plan.Disks...), nil
}
