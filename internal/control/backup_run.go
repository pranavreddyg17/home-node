package control

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
)

// RunApprovedBackupPassword takes ownership of password bytes and clears them
// on every path. It creates a sealed read-only credential before admission and
// retains that descriptor until maintenance and worker completion return. HTTP
// callers must supply bounded byte input, never persist it or put it in a grant.
func (s *Server) RunApprovedBackupPassword(ctx context.Context, actor identity.Session, grant string, body []byte, requestKey, release string, catalogVersion int64, password []byte, launch func(context.Context, backup.Launch, *os.File) error, cleanup func(context.Context, backup.Cleanup, *os.File) error) (jobID, snapshotID string, resultErr error) {
	defer clear(password)
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	credential, err := backup.CreateRepositoryPassword(password)
	clear(password)
	if err != nil {
		return "", "", err
	}
	defer func() { resultErr = errors.Join(resultErr, credential.Close()) }()
	return s.RunApprovedBackup(ctx, actor, grant, body, requestKey, release, catalogVersion, credential, launch, cleanup)
}

// RunApprovedBackup joins owner approval admission to isolated maintenance.
// Release/catalog/worker delivery come from trusted installed configuration. The
// caller owns the sealed credential and must use a server-owned work context.
// No maintenance token is returned to the browser. Dispatch is never retried.
func (s *Server) RunApprovedBackup(ctx context.Context, actor identity.Session, grant string, body []byte, requestKey, release string, catalogVersion int64, credential *os.File, launch func(context.Context, backup.Launch, *os.File) error, cleanup func(context.Context, backup.Cleanup, *os.File) error) (string, string, error) {
	if s.config.BackupRepositoryID == "" || s.config.Runtime == nil || launch == nil || cleanup == nil || credential == nil {
		return "", "", backup.ErrManifest
	}
	// Qualify preliminary metadata before consuming approval or closing admission.
	validation := "backup-validation-identity"
	if _, err := backup.EncodeLaunch(backup.Launch{Version: 2, JobID: validation, DeviceID: actor.Device.ID, ManagementToken: validation, Release: release, CatalogVersion: catalogVersion}); err != nil {
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
	snapshot, err := backup.RunAdmittedLaunchedMaintenance(ctx, s.Store, actor.Device.ID, token, job.ID, release, catalogVersion, credential, s.Workloads, launch, cleanup)
	return job.ID, snapshot, err
}
