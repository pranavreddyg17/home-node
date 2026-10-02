package control

import (
	"context"
	"database/sql"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
	"io"
	"net/http"
)

func (s *Server) approvalRoutes() {
	s.mux.Handle("POST /api/v1/backups/{id}/resume/approval", s.require("admin", false, http.HandlerFunc(s.backupResumeApproval)))
	s.mux.Handle("POST /api/v1/backups/{id}/resume", s.require("admin", false, http.HandlerFunc(s.backupResume)))
	s.mux.Handle("POST /api/v1/backups/approval", s.require("admin", false, http.HandlerFunc(s.backupApproval)))
	s.mux.Handle("POST /api/v1/ai/conversations/{id}/delete/approval", s.require("ai", false, http.HandlerFunc(s.conversationDeleteApproval)))
	s.mux.Handle("POST /api/v1/apps/{workload}/actions/approval", s.require("admin", false, http.HandlerFunc(s.appApproval)))
	s.mux.Handle("POST /api/v1/devices/pair/approval", s.require("admin", false, http.HandlerFunc(s.pairApproval)))
	s.mux.Handle("POST /api/v1/devices/{id}/revoke/approval", s.require("admin", false, http.HandlerFunc(s.revokeApproval)))
	s.mux.Handle("POST /api/v1/auth/approval/finish", s.require("", false, http.HandlerFunc(s.approvalFinish)))
}

func (s *Server) backupApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	if s.config.BackupRepositoryID == "" {
		fail(w, http.StatusServiceUnavailable, "BACKUP_UNAVAILABLE", "A trusted backup repository has not been configured.")
		return
	}
	resources, err := identity.BackupApprovalResources(body, s.config.BackupRepositoryID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.authError(w, err)
		return
	}
	options, token, binding, err := s.Identity.BeginApproval(r.Context(), actor(r), "backup.create", resources, body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}

func approvalBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	if err != nil || len(body) > 4096 {
		fail(w, 400, "INVALID_REQUEST", "The approval request is malformed or too large.")
		return nil, false
	}
	return body, true
}

func (s *Server) pairApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	options, token, binding, err := s.Identity.BeginPairApproval(r.Context(), actor(r), body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}

func (s *Server) revokeApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	options, token, binding, err := s.Identity.BeginRevokeApproval(r.Context(), actor(r), r.PathValue("id"), body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}

func (s *Server) approvalFinish(w http.ResponseWriter, r *http.Request) {
	grant, err := s.Identity.FinishApproval(r.Context(), actor(r), r.Header.Get("X-Approval-Challenge"), s.config.PolicyGeneration, r)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"approvalToken": grant})
}

func (s *Server) appApprovalResources(ctx context.Context, name, key string) ([]string, []string, error) {
	if len(key) < 16 || len(key) > 128 {
		return nil, nil, workload.ErrInvalid
	}
	var snapshot []string
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		snapshot, err = s.Workloads.AppApprovalResources(tx, name)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	resources := append(append([]string{}, snapshot...), state.Hash(key))
	return snapshot, resources, nil
}

func (s *Server) appApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	action, err := identity.ParseAppApprovalAction(body)
	if err != nil {
		s.authError(w, err)
		return
	}
	if s.config.Runtime == nil {
		workloadError(w, workload.ErrUnavailable)
		return
	}
	_, resources, err := s.appApprovalResources(r.Context(), r.PathValue("workload"), r.Header.Get("Idempotency-Key"))
	if err != nil {
		workloadError(w, err)
		return
	}
	options, token, binding, err := s.Identity.BeginApproval(r.Context(), actor(r), "app."+action, resources, body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}

func (s *Server) conversationDeleteResources(id, key string) ([]string, error) {
	if len(key) < 16 || len(key) > 128 {
		return nil, workload.ErrInvalid
	}
	return []string{id, state.Hash(key)}, nil
}

func (s *Server) conversationDeleteApproval(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	if !identity.ValidEmptyApprovalBody(body) {
		s.authError(w, identity.ErrDenied)
		return
	}
	id := r.PathValue("id")
	resources, err := s.conversationDeleteResources(id, r.Header.Get("Idempotency-Key"))
	if err != nil {
		workloadError(w, err)
		return
	}
	var exists, active int
	if err := s.Store.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM conversations WHERE id=?", id).Scan(&exists); err != nil || exists != 1 {
		workloadError(w, workload.ErrInvalid)
		return
	}
	if err := s.Store.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM generations WHERE conversation_id=? AND state IN('staging','queued','running','cancelling')", id).Scan(&active); err != nil || active > 0 {
		workloadError(w, workload.ErrConflict)
		return
	}
	options, token, binding, err := s.Identity.BeginApproval(r.Context(), actor(r), "ai.delete", resources, body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": options, "challengeToken": token, "expiresAt": binding.ExpiresAt})
}
