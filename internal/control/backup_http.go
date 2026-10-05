package control

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"net/http"
	"os"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

const backupCredentialMediaType = "application/vnd.homenode.backup-credential"

// BackupExecutionConfig is installed host configuration, never request input.
// Callbacks use the protected activated worker socket and await exact completion.
type BackupExecutionConfig struct {
	Release         string
	CatalogVersion  int64
	Launch          func(context.Context, backup.Launch, *os.File) error
	Cleanup         func(context.Context, backup.Cleanup, *os.File) error
	SnapshotPage    func(context.Context, backup.SnapshotPageRequest, *os.File) (backup.SnapshotPage, error)
	SnapshotPreview func(context.Context, backup.SnapshotPreviewRequest, *os.File) (backup.SnapshotPreview, error)
}

func validateBackupExecution(config Config) error {
	execution := config.BackupExecution
	if execution == nil {
		return nil
	}
	if config.BackupRepositoryID == "" || config.Runtime == nil || config.Development || execution.Launch == nil || execution.Cleanup == nil {
		return backup.ErrManifest
	}
	sentinel := "backup-validation-identity"
	_, err := backup.EncodeLaunch(backup.Launch{Version: 2, JobID: sentinel, DeviceID: sentinel, ManagementToken: sentinel, Release: execution.Release, CatalogVersion: execution.CatalogVersion})
	return err
}

func (s *Server) backupCreate(w http.ResponseWriter, r *http.Request) {
	execution := s.config.BackupExecution
	if execution == nil || s.config.BackupRepositoryID == "" {
		fail(w, 503, "BACKUP_UNAVAILABLE", "Backup execution has not been configured.")
		return
	}
	if r.URL.RawQuery != "" {
		fail(w, 400, "INVALID_REQUEST", "Send backup metadata and credential in the request body.")
		return
	}
	body, password, err := readBackupCredentialRequest(r.Body, s.config.BackupRepositoryID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		fail(w, 400, "INVALID_REQUEST", "The backup request is malformed or too large.")
		return
	}
	defer clear(password)
	job, err := s.StartApprovedBackupPassword(r.Context(), actor(r), r.Header.Get("X-Action-Approval"), body, r.Header.Get("Idempotency-Key"), execution.Release, execution.CatalogVersion, password, execution.Launch, execution.Cleanup)
	if errors.Is(err, errBackupTaskUnavailable) {
		fail(w, 409, "BACKUP_BUSY", "Backup work is active or the server is stopping.")
		return
	}
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job, "status": "preparing"})
}

func (s *Server) backupConfiguration(w http.ResponseWriter, r *http.Request) {
	enabled := s.config.BackupExecution != nil && s.config.BackupRepositoryID != ""
	repository := ""
	if enabled {
		repository = s.config.BackupRepositoryID
	}
	availability := "not-configured"
	if enabled {
		availability = "paused"
		if s.backupTasks != nil {
			s.backupTasks.mu.Lock()
			idle := !s.backupTasks.active && !s.backupTasks.stopping
			s.backupTasks.mu.Unlock()
			if idle {
				admissionErr := s.Store.Transaction(r.Context(), state.RequireAdmission)
				switch {
				case admissionErr == nil:
					availability = "available"
				case errors.Is(admissionErr, state.ErrMaintenance):
					availability = "paused"
				default:
					availability = "unavailable"
				}
			}
		}
	}
	// This sampled hint grants no authority and does not assert drive presence.
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "repositoryId": repository, "availability": availability})
}
