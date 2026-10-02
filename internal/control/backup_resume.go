package control

import (
	"context"
	"errors"
	"net/http"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
)

func (s *Server) StartApprovedBackupResume(ctx context.Context, actor identity.Session, grant string, body []byte, id, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.config.Runtime == nil || s.backupTasks == nil {
		return "", errBackupTaskUnavailable
	}
	err := s.backupTasks.start(func(workContext context.Context) (func(context.Context) error, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, job, err := s.Identity.AuthorizeBackupResumeApproved(workContext, actor, grant, body, id, key, s.config.PolicyGeneration)
		if err != nil {
			return nil, err
		}
		return func(operation context.Context) error {
			return backup.RecoverReleasedBackupMaintenance(operation, s.Store, token, job.ID, actor.Device.ID, s.Workloads)
		}, nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Server) backupResumeApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	resources, err := identity.BackupRecoveryApprovalResources(body, r.PathValue("id"), r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.authError(w, err)
		return
	}
	options, token, binding, err := s.Identity.BeginApproval(r.Context(), actor(r), "backup.resume", resources, body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}

func (s *Server) backupResume(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	id, err := s.StartApprovedBackupResume(r.Context(), actor(r), r.Header.Get("X-Action-Approval"), body, r.PathValue("id"), r.Header.Get("Idempotency-Key"))
	if errors.Is(err, errBackupTaskUnavailable) {
		fail(w, 409, "BACKUP_BUSY", "Backup work is active, unavailable, or the server is stopping.")
		return
	}
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": id, "status": "restoring"})
}
