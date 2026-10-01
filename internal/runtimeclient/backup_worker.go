package runtimeclient

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// BackupWorkerConfig comes from trusted installed configuration, never dispatch
// data. A zero listener UID explicitly selects the root-created systemd socket.
// Release/catalog values must come from verified installed release metadata.
type BackupWorkerConfig struct {
	ManagementSocket, DiskSocket, Release string
	ControllerListenerUID                 uint32
	CatalogVersion                        int64
	Policy                                backup.RestorePolicy
}

// RunDispatchedBackup binds authenticated job data to fixed installed bridges.
// Caller must authenticate dispatch, retain barriers, and provide exclusively
// owned empty staging and repository handles. It does not release authority or
// dispose staging. A snapshot ID may accompany an acknowledgement error.
func RunDispatchedBackup(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig, staging *os.Root, repository backup.RecoveryPublisher) (backup.BackupResult, error) {
	if _, err := backup.EncodeDispatch(dispatch); err != nil {
		return backup.BackupResult{}, err
	}
	if config.Release != dispatch.Release || config.CatalogVersion != dispatch.CatalogVersion || config.Policy.MinimumCatalogVersion < 1 || config.CatalogVersion < config.Policy.MinimumCatalogVersion || staging == nil || repository == nil || !filepath.IsAbs(config.ManagementSocket) || filepath.Clean(config.ManagementSocket) != config.ManagementSocket || !filepath.IsAbs(config.DiskSocket) || filepath.Clean(config.DiskSocket) != config.DiskSocket {
		return backup.BackupResult{}, backup.ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return backup.BackupResult{}, err
	}
	management := newOwnedMaintenanceApps(config.ManagementSocket, config.ControllerListenerUID, dispatch.ManagementToken, dispatch.JobID, dispatch.DeviceID)
	defer management.Close()
	snapshot, err := backup.RunPrivateBackup(ctx, management, NewDiskClient(config.DiskSocket), repository, dispatch.DeviceID, dispatch.ManagementToken, dispatch.RuntimeToken, staging, config.Release, config.CatalogVersion, config.Policy)
	return backup.BackupResult{JobID: dispatch.JobID, SnapshotID: snapshot}, err
}
