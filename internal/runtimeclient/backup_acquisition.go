package runtimeclient

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/backup"
)

type BackupAcquisitionManagement interface {
	ConfirmFreezing(context.Context, string, string, string) error
	AttachRuntimeRoot(context.Context, string, string, string) error
}

// AcquireOwnedBackupRuntime runs under the isolated backup identity. A failed
// attachment retains the known root token for explicit reconciliation; it never
// releases runtime authority after an uncertain acknowledgement or retries.
func AcquireOwnedBackupRuntime(ctx context.Context, management BackupAcquisitionManagement, root backup.MaintenanceRoot, token, jobID, device string) (string, error) {
	if management == nil || root == nil || !maintenanceID.MatchString(token) || !maintenanceID.MatchString(jobID) || !maintenanceID.MatchString(device) {
		return "", ErrMaintenance
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := management.ConfirmFreezing(ctx, token, jobID, device); err != nil {
		return "", err
	}
	rootToken, err := root.BeginRuntimeMaintenanceForJob(ctx, jobID)
	if err != nil {
		return "", err
	}
	if !maintenanceID.MatchString(rootToken) {
		return "", ErrMaintenance
	}
	if err := management.AttachRuntimeRoot(ctx, token, device, rootToken); err != nil {
		return rootToken, err
	}
	return rootToken, nil
}

// AcquireLaunchedBackupRuntime converts only a qualified preliminary launch
// after worker-owned acquisition. A dispatch returned with an error is retained
// reconciliation state, never permission to proceed with backup publication.
func AcquireLaunchedBackupRuntime(ctx context.Context, launch backup.Launch, management BackupAcquisitionManagement, root backup.MaintenanceRoot) (backup.Dispatch, error) {
	if _, err := backup.EncodeLaunch(launch); err != nil {
		return backup.Dispatch{}, err
	}
	token, err := AcquireOwnedBackupRuntime(ctx, management, root, launch.ManagementToken, launch.JobID, launch.DeviceID)
	if token == "" {
		return backup.Dispatch{}, err
	}
	dispatch, conversionErr := launch.AcquiredDispatch(token)
	if conversionErr != nil {
		return backup.Dispatch{}, conversionErr
	}
	return dispatch, err
}
