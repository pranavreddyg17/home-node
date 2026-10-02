package runtimeclient

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/backup"
)

type BackupReleaseManagement interface {
	ConfirmRuntimeRelease(context.Context, string, string, string) error
	RecordRuntimeReleased(context.Context, string, string, string) error
}

// ReleaseOwnedBackupRuntime runs as the isolated backup peer only after worker
// completion has been acknowledged. Failed release/record leaves management's
// root checkpoint intact for recovery; this operation never resumes apps.
func ReleaseOwnedBackupRuntime(ctx context.Context, management BackupReleaseManagement, root backup.MaintenanceRoot, token, device, rootToken string) error {
	if management == nil || root == nil || !maintenanceID.MatchString(token) || !maintenanceID.MatchString(device) || !maintenanceID.MatchString(rootToken) {
		return ErrMaintenance
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := management.ConfirmRuntimeRelease(ctx, token, device, rootToken); err != nil {
		return err
	}
	if err := root.EndRuntimeMaintenance(ctx, rootToken); err != nil {
		return err
	}
	return management.RecordRuntimeReleased(ctx, token, device, rootToken)
}
