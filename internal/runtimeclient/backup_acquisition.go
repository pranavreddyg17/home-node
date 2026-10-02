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
