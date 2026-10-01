package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// RecoveryPublisher is implemented by the authenticated encrypted Repository.
// The caller must retain exclusive ownership of the repository and staging area.
type RecoveryPublisher interface {
	Snapshot(context.Context, *os.File, Manifest, RestorePolicy) (string, error)
}

type BackupResult struct {
	JobID      string
	SnapshotID string
}

// RunBackup stages the owned recovery inventory and publishes it while both
// maintenance barriers are held. A snapshot ID can accompany a cleanup error:
// publication succeeded, but source admission has not safely reopened. This
// private entry point requires trusted bridges and an exclusively owned empty
// staging root; filesystem consistency qualification remains their responsibility.
// It leaves staging intact for the caller's protected disposal/recovery policy.
func RunBackup(ctx context.Context, store *state.Store, device string, apps MaintenanceApps, runtime MaintenanceRoot, disks MaintenanceDisks, staging *os.Root, repository RecoveryPublisher, release string, catalogVersion int64, policy RestorePolicy) (result BackupResult, resultErr error) {
	if staging == nil || disks == nil || repository == nil {
		return result, ErrManifest
	}
	// Pin the directory passed to publication before any maintenance effects.
	directory, err := staging.Open(".")
	if err != nil {
		return result, err
	}
	defer directory.Close()
	var manifest Manifest
	result.JobID, resultErr = RunMaintenance(ctx, store, device, apps, runtime,
		func(ctx context.Context, managementToken, runtimeToken string) error {
			var err error
			manifest, err = StageRecoverySet(ctx, store, disks, managementToken, runtimeToken, staging, release, catalogVersion, policy)
			return err
		},
		func(ctx context.Context, managementToken, runtimeToken string) error {
			job, err := store.InspectMaintenanceJob(ctx, managementToken)
			if err != nil {
				return err
			}
			if job.Phase != "publishing" || job.RootToken != runtimeToken {
				return state.ErrMaintenanceOwner
			}
			result.SnapshotID, err = repository.Snapshot(ctx, directory, manifest, policy)
			return err
		})
	return result, resultErr
}
