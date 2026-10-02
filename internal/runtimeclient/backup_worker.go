package runtimeclient

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// BackupWorkerConfig comes from trusted installed configuration, never dispatch
// data. A zero listener UID explicitly selects the root-created systemd socket.
// Release/catalog values must come from verified installed release metadata.
type BackupWorkerConfig struct {
	ManagementSocket, DiskSocket, Release string
	MaintenanceSocket                     string
	StagingParent                         string
	RepositoryTarget                      backup.Target
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
	return validateBackupWorkerInputs(ctx, dispatch.Release, dispatch.CatalogVersion, config)
}

func validateBackupWorkerInputs(ctx context.Context, release string, catalogVersion int64, config BackupWorkerConfig) error {
	if config.Release != release || config.CatalogVersion != catalogVersion || config.Policy.MinimumCatalogVersion < 1 || config.CatalogVersion < config.Policy.MinimumCatalogVersion || !filepath.IsAbs(config.ManagementSocket) || filepath.Clean(config.ManagementSocket) != config.ManagementSocket || !filepath.IsAbs(config.DiskSocket) || filepath.Clean(config.DiskSocket) != config.DiskSocket {
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

// RunRegisteredDispatchedBackup opens only the configured registered external
// destination, authenticates the encrypted repository, and retains its kernel
// lease/credential handle through leased staging and publication. The password
// must come from trusted local credential handoff, never dispatch or command
// arguments. Caller owns clearing its password bytes after return.
func RunRegisteredDispatchedBackup(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig, password []byte) (result backup.BackupResult, resultErr error) {
	if err := validateBackupWorker(ctx, dispatch, config); err != nil {
		return result, err
	}
	if !filepath.IsAbs(config.StagingParent) || filepath.Clean(config.StagingParent) != config.StagingParent {
		return result, backup.ErrManifest
	}
	if err := config.RepositoryTarget.Validate(); err != nil {
		return result, err
	}
	repository, err := backup.OpenRepository(ctx, config.RepositoryTarget, password)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, repository.Close()) }()
	return RunLeasedDispatchedBackup(ctx, dispatch, config, repository)
}

// RunCredentialedDispatchedBackup consumes and closes the supplied credential
// descriptor on every path. It clears its temporary password copy after use.
// Credential transport must independently authenticate trusted local handoff;
// ordinary dispatch packets continue to reject descriptors and secrets.
func RunCredentialedDispatchedBackup(ctx context.Context, dispatch backup.Dispatch, config BackupWorkerConfig, credential *os.File) (result backup.BackupResult, resultErr error) {
	if credential == nil {
		return result, backup.ErrRepository
	}
	defer func() { resultErr = errors.Join(resultErr, credential.Close()) }()
	if err := validateBackupWorker(ctx, dispatch, config); err != nil {
		return result, err
	}
	if !filepath.IsAbs(config.StagingParent) || filepath.Clean(config.StagingParent) != config.StagingParent {
		return result, backup.ErrManifest
	}
	if err := config.RepositoryTarget.Validate(); err != nil {
		return result, err
	}
	password, err := backup.ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return result, err
	}
	defer clear(password)
	return RunRegisteredDispatchedBackup(ctx, dispatch, config, password)
}

// ServeRegisteredBackupWorker owns the separate credential listener and binds
// each admitted job to the configured registered worker. The caller must supply
// trusted installed configuration and an exclusively owned protected listener.
// Durable controller outcomes, not socket closure, establish publication.
func ServeRegisteredBackupWorker(ctx context.Context, listener net.Listener, controllerUID uint32, config BackupWorkerConfig) error {
	if !filepath.IsAbs(config.MaintenanceSocket) || filepath.Clean(config.MaintenanceSocket) != config.MaintenanceSocket {
		if listener != nil {
			listener.Close()
		}
		return backup.ErrManifest
	}
	return backup.ServeAcknowledgedCredentialWorker(ctx, listener, controllerUID, func(operation context.Context, request backup.WorkerRequest, credential *os.File) error {
		root := NewMaintenance(config.MaintenanceSocket)
		defer root.client.CloseIdleConnections()
		if request.Launch != nil && request.Cleanup == nil {
			_, _, err := RunCredentialedLaunchedBackup(operation, *request.Launch, config, credential, root)
			return err
		}
		if request.Cleanup != nil && request.Launch == nil {
			return RunRegisteredBackupCleanup(operation, *request.Cleanup, config, root)
		}
		return backup.ErrManifest
	})
}

// RunCredentialedLaunchedBackup executes as the isolated backup identity. It
// retains acquired dispatch state on later errors and never releases the runtime
// barrier while publication or callback completion may still be active. Separate
// acknowledged cleanup/recovery must run before apps resume.
func RunCredentialedLaunchedBackup(ctx context.Context, launch backup.Launch, config BackupWorkerConfig, credential *os.File, root backup.MaintenanceRoot) (dispatch backup.Dispatch, result backup.BackupResult, resultErr error) {
	if credential == nil {
		return dispatch, result, backup.ErrRepository
	}
	defer func() { resultErr = errors.Join(resultErr, credential.Close()) }()
	if _, err := backup.EncodeLaunch(launch); err != nil {
		return dispatch, result, err
	}
	if err := validateBackupWorkerInputs(ctx, launch.Release, launch.CatalogVersion, config); err != nil {
		return dispatch, result, err
	}
	if root == nil || !filepath.IsAbs(config.StagingParent) || filepath.Clean(config.StagingParent) != config.StagingParent {
		return dispatch, result, backup.ErrManifest
	}
	if err := config.RepositoryTarget.Validate(); err != nil {
		return dispatch, result, err
	}
	password, err := backup.ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return dispatch, result, err
	}
	defer clear(password)
	// Authenticate and pin the registered destination before runtime acquisition.
	// Retain this exact repository lease through publication; never reopen by path.
	repository, err := backup.OpenRepository(ctx, config.RepositoryTarget, password)
	clear(password)
	if err != nil {
		return dispatch, result, errors.Join(backup.ErrLaunchRepositoryAdmission, err)
	}
	defer func() { resultErr = errors.Join(resultErr, repository.Close()) }()

	management := newOwnedMaintenanceApps(config.ManagementSocket, config.ControllerListenerUID, launch.ManagementToken, launch.JobID, launch.DeviceID)
	defer management.Close()
	dispatch, err = AcquireLaunchedBackupRuntime(ctx, launch, management, root)
	if err != nil {
		return dispatch, result, err
	}
	result, err = RunLeasedDispatchedBackup(ctx, dispatch, config, repository)
	return dispatch, result, err
}

// RunRegisteredBackupCleanup runs on the isolated backup peer after publication
// callback completion is acknowledged. No repository access/password is needed
// here; the private management preflight independently proves stopped ownership.
func RunRegisteredBackupCleanup(ctx context.Context, cleanup backup.Cleanup, config BackupWorkerConfig, root backup.MaintenanceRoot) error {
	if _, err := backup.EncodeCleanup(cleanup); err != nil {
		return err
	}
	if root == nil || !filepath.IsAbs(config.ManagementSocket) || filepath.Clean(config.ManagementSocket) != config.ManagementSocket {
		return backup.ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	management := newOwnedMaintenanceApps(config.ManagementSocket, config.ControllerListenerUID, cleanup.ManagementToken, cleanup.JobID, cleanup.DeviceID)
	defer management.Close()
	return ReleaseOwnedBackupRuntime(ctx, management, root, cleanup.ManagementToken, cleanup.DeviceID, cleanup.RuntimeToken)
}
