package runtimeclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// BackupWorkerConfig comes from trusted installed configuration, never dispatch
// data. A zero listener UID explicitly selects the root-created systemd socket.
// Release/catalog values must come from verified installed release metadata.
type BackupWorkerConfig struct {
	ManagementSocket, DiskSocket, Release string
	StagingParent                         string
	ControllerListenerUID                 uint32
	CatalogVersion                        int64
	Policy                                backup.RestorePolicy
}

// RunDispatchedBackup binds authenticated job data to fixed installed bridges.
// Caller must authenticate dispatch, retain barriers, and provide exclusively
// owned empty staging and repository handles. It does not release authority or
// dispose staging. A snapshot ID may accompany an acknowledgement error.
func RunDispatchedBackup(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig, staging *os.Root, repository backup.RecoveryPublisher) (backup.BackupResult, error) {
	if err := validateBackupWorker(ctx, dispatch, config); err != nil {
		return backup.BackupResult{}, err
	}
	if staging == nil || repository == nil {
		return backup.BackupResult{}, backup.ErrManifest
	}
	management := newOwnedMaintenanceApps(config.ManagementSocket, config.ControllerListenerUID, dispatch.ManagementToken, dispatch.JobID, dispatch.DeviceID)
	defer management.Close()
	snapshot, err := backup.RunPrivateBackup(ctx, management, NewDiskClient(config.DiskSocket), repository, dispatch.DeviceID, dispatch.ManagementToken, dispatch.RuntimeToken, staging, config.Release, config.CatalogVersion, config.Policy)
	return backup.BackupResult{JobID: dispatch.JobID, SnapshotID: snapshot}, err
}

func validateBackupWorker(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig) error {
	if _, err := backup.EncodeDispatch(dispatch); err != nil {
		return err
	}
	if config.Release != dispatch.Release || config.CatalogVersion != dispatch.CatalogVersion || config.Policy.MinimumCatalogVersion < 1 || config.CatalogVersion < config.Policy.MinimumCatalogVersion || !filepath.IsAbs(config.ManagementSocket) || filepath.Clean(config.ManagementSocket) != config.ManagementSocket || !filepath.IsAbs(config.DiskSocket) || filepath.Clean(config.DiskSocket) != config.DiskSocket {
		return backup.ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

// RunLeasedDispatchedBackup creates private job staging beneath a fixed
// installed parent and retains its exclusive lease until all worker effects
// finish. Closing never deletes staged data or changes maintenance barriers.
func RunLeasedDispatchedBackup(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig, repository backup.RecoveryPublisher) (result backup.BackupResult, resultErr error) {
	if err := validateBackupWorker(ctx, dispatch, config); err != nil {
		return result, err
	}
	if repository == nil || !filepath.IsAbs(config.StagingParent) || filepath.Clean(config.StagingParent) != config.StagingParent {
		return result, backup.ErrManifest
	}
	lease, err := backup.OpenJobStaging(ctx, config.StagingParent, dispatch.JobID)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, lease.Close()) }()
	return RunDispatchedBackup(ctx, dispatch, config, lease.Root(), repository)
}
