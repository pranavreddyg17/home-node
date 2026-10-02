package control

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
)

// RunApprovedBackup joins owner approval admission to isolated maintenance.
// Release/catalog/root/delivery come from trusted installed configuration. The
// caller owns the sealed credential and must use a server-owned work context.
// No maintenance token is returned to the browser. Dispatch is never retried.
func (s *Server) RunApprovedBackup(ctx context.Context, actor identity.Session, grant string, body []byte, requestKey, release string, catalogVersion int64, credential *os.File, root backup.MaintenanceRoot, deliver func(context.Context, backup.Dispatch, *os.File) error) (string, string, error) {
	if s.config.BackupRepositoryID == "" || s.config.Runtime == nil || root == nil || deliver == nil || credential == nil {
		return "", "", backup.ErrManifest
	}
	// Reuse the exact dispatch metadata validator before committing admission.
	validation := "backup-validation-identity"
	if _, err := backup.EncodeDispatch(backup.Dispatch{Version: 1, JobID: validation, DeviceID: actor.Device.ID, ManagementToken: validation, RuntimeToken: validation, Release: release, CatalogVersion: catalogVersion}); err != nil {
		return "", "", err
	}
	password, err := backup.ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return "", "", err
	}
	clear(password)
	token, job, err := s.Identity.AdmitBackupApproved(ctx, actor, grant, body, s.config.BackupRepositoryID, requestKey, s.config.PolicyGeneration)
	if err != nil {
		return "", "", err
	}
	return backup.RunAdmittedDispatchedMaintenance(ctx, s.Store, actor.Device.ID, token, job.ID, release, catalogVersion, credential, s.Workloads, root, deliver)
}
