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
// staging root. Each app disk must pass read-only ext4 qualification before copying;
// the bridges must still establish clean guest shutdown and exclude other writers.
// It leaves staging intact for the caller's protected disposal/recovery policy.
func RunBackup(ctx context.Context, store *state.Store, device string, apps MaintenanceApps, runtime MaintenanceRoot, disks MaintenanceDisks, staging *os.Root, repository RecoveryPublisher, release string, catalogVersion int64, policy RestorePolicy) (result BackupResult, resultErr error) {
	if disks == nil {
		return result, ErrManifest
	}
	return runBackup(ctx, store, device, apps, runtime, QualifiedMaintenanceDisks{Source: disks}, staging, repository, release, catalogVersion, policy)
}

// runBackup separates orchestration from disk admission for focused tests.
// Production callers use RunBackup, which always qualifies source filesystems.
func runBackup(ctx context.Context, store *state.Store, device string, apps MaintenanceApps, runtime MaintenanceRoot, disks MaintenanceDisks, staging *os.Root, repository RecoveryPublisher, release string, catalogVersion int64, policy RestorePolicy) (result BackupResult, resultErr error) {
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
			if err = store.ClaimBackupPublication(ctx, managementToken, job.ID, device); err != nil {
				return err
			}
			result.SnapshotID, err = repository.Snapshot(ctx, directory, manifest, policy)
			if err != nil {
				return err
			}
			if !repositoryPattern.MatchString(result.SnapshotID) {
				result.SnapshotID = ""
				return ErrRepository
			}
			return store.RecordBackupPublished(ctx, managementToken, job.ID, device, result.SnapshotID)
		})
	return result, resultErr
}
