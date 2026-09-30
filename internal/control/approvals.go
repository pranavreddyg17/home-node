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
	s.mux.Handle("POST /api/v1/apps/{workload}/actions/approval", s.require("admin", false, http.HandlerFunc(s.appApproval)))
	s.mux.Handle("POST /api/v1/devices/pair/approval", s.require("admin", false, http.HandlerFunc(s.pairApproval)))
	s.mux.Handle("POST /api/v1/devices/{id}/revoke/approval", s.require("admin", false, http.HandlerFunc(s.revokeApproval)))
	s.mux.Handle("POST /api/v1/auth/approval/finish", s.require("", false, http.HandlerFunc(s.approvalFinish)))
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
