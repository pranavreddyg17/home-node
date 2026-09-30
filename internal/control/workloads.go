package control

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func (s *Server) workloadRoutes() {
	s.mux.Handle("GET /api/v1/ai/conversations", s.require("ai", false, http.HandlerFunc(s.conversations)))
	s.mux.Handle("POST /api/v1/ai/conversations", s.require("ai", false, http.HandlerFunc(s.createConversation)))
	s.mux.Handle("GET /api/v1/ai/conversations/{id}/history", s.require("ai", false, http.HandlerFunc(s.history)))
	s.mux.Handle("GET /api/v1/ai/conversations/{id}/export", s.require("ai", false, http.HandlerFunc(s.exportConversation)))
	s.mux.Handle("POST /api/v1/ai/conversations/{id}/delete", s.require("ai", false, http.HandlerFunc(s.deleteConversation)))
	s.mux.Handle("POST /api/v1/ai/generations", s.require("ai", false, http.HandlerFunc(s.createGeneration)))
	s.mux.Handle("GET /api/v1/ai/generations/{id}", s.require("ai", false, http.HandlerFunc(s.generation)))
	s.mux.Handle("POST /api/v1/ai/generations/{id}/cancel", s.require("ai", false, http.HandlerFunc(s.cancelGeneration)))
	s.mux.Handle("GET /api/v1/jobs", s.require("jobs", false, http.HandlerFunc(s.jobs)))
	s.mux.Handle("POST /api/v1/jobs", s.require("jobs", false, http.HandlerFunc(s.createJob)))
	s.mux.Handle("POST /api/v1/jobs/{id}/cancel", s.require("jobs", false, http.HandlerFunc(s.cancelJob)))
	s.mux.Handle("GET /api/v1/apps", s.require("", false, http.HandlerFunc(s.apps)))
	s.mux.Handle("POST /api/v1/apps/{workload}/actions", s.require("admin", false, http.HandlerFunc(s.appAction)))
	s.mux.Handle("GET /api/v1/operations/{id}", s.require("", false, http.HandlerFunc(s.operation)))
	s.mux.Handle("POST /api/v1/transfers", s.require("files", false, http.HandlerFunc(s.createTransfer)))
	s.mux.Handle("GET /api/v1/transfers/{id}", s.require("files", false, http.HandlerFunc(s.transfer)))
	s.mux.Handle("POST /api/v1/transfers/{id}/chunks", s.require("files", false, http.HandlerFunc(s.upload)))
	s.mux.Handle("POST /api/v1/transfers/{id}/finalize", s.require("files", false, http.HandlerFunc(s.finalize)))
	s.mux.Handle("POST /api/v1/transfers/{id}/cancel", s.require("files", false, http.HandlerFunc(s.cancelTransfer)))
	s.mux.Handle("GET /api/v1/files", s.require("files", false, http.HandlerFunc(s.files)))
	s.mux.Handle("POST /api/v1/files/{id}/actions", s.require("files", false, http.HandlerFunc(s.changeFile)))
	s.mux.Handle("GET /api/v1/files/{id}/download", s.require("files", false, http.HandlerFunc(s.download)))
}
func workloadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, state.ErrMaintenance):
		fail(w, 409, "POLICY_DENIED", "Host maintenance is in progress. Existing work may drain; new work is blocked until maintenance safely finishes.")
	case errors.Is(err, workload.ErrUnavailable):
		fail(w, 503, "WORKLOAD_UNAVAILABLE", "Start the required app on a qualified Linux host with verified guest images.")
	case errors.Is(err, workload.ErrInvalid):
		fail(w, 400, "INVALID_REQUEST", "Check the name, file size, checksum, or action.")
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "NOT_FOUND", "The resource is unavailable to this device.")
	default:
		fail(w, 409, "OPERATION_CONFLICT", "The operation could not complete. Refresh its status before retrying.")
	}
}
func (s *Server) apps(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.Apps(r.Context())
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"apps": items, "runtimeConfigured": s.config.Runtime != nil})
}
func (s *Server) appAction(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	action, err := identity.ParseAppApprovalAction(body)
	if err != nil {
		s.authError(w, err)
		return
	}
	grant := r.Header.Get("X-Action-Approval")
	if grant == "" {
		fail(w, 403, "APPROVAL_REQUIRED", "Approve this app action with your passkey.")
		return
	}
	key, name := r.Header.Get("Idempotency-Key"), r.PathValue("workload")
	snapshot, resources, err := s.appApprovalResources(r.Context(), name, key)
	if err != nil {
		workloadError(w, err)
		return
	}
	var op workload.Operation
	err = s.Identity.ConsumeApproval(r.Context(), actor(r), grant, "app."+action, resources, body, s.config.PolicyGeneration, func(tx *sql.Tx) error {
		var err error
		op, err = s.Workloads.AppActionAtResourcesInTransaction(tx, actor(r).Device.ID, key, name, action, snapshot)
		return err
	})
	if err != nil {
		if errors.Is(err, identity.ErrDenied) {
			s.authError(w, err)
		} else {
			workloadError(w, err)
		}
		return
	}
	writeJSON(w, 202, op)
}
func (s *Server) operation(w http.ResponseWriter, r *http.Request) {
	op, err := s.Workloads.Operation(r.Context(), actor(r).Device.ID, r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, op)
}
func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	if !decode(w, r, &body) {
		return
	}
	item, err := s.Workloads.CreateTransfer(r.Context(), actor(r).Device.ID, body.Name, body.Size, body.SHA256)
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) transfer(w http.ResponseWriter, r *http.Request) {
	item, err := s.Workloads.Transfer(r.Context(), actor(r).Device.ID, r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Offset int64  `json:"offset"`
		Data   []byte `json:"data"`
		SHA256 string `json:"sha256"`
	}
	if !decode(w, r, &body) {
		return
	}
	item, err := s.Workloads.Upload(r.Context(), actor(r).Device.ID, r.PathValue("id"), body.Offset, body.Data, body.SHA256)
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) finalize(w http.ResponseWriter, r *http.Request) {
	item, err := s.Workloads.Finalize(r.Context(), actor(r).Device.ID, r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) cancelTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	item, err := s.Workloads.CancelTransfer(r.Context(), actor(r).Device.ID, r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 202, item)
}
func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.Files(r.Context())
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) changeFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Workloads.ChangeFile(r.Context(), actor(r).Device.ID, r.PathValue("id"), body.Action, body.Name); err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"updated": true})
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file, err := s.Workloads.File(r.Context(), id)
	if err != nil {
		workloadError(w, err)
		return
	}
	if file.TrashUntil != nil {
		workloadError(w, workload.ErrConflict)
		return
	}
	// Fetch one checked chunk before committing successful download headers.
	data, err := s.Workloads.Download(r.Context(), id, 0)
	if err != nil {
		workloadError(w, err)
		return
	}
	if int64(len(data)) == file.Size {
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			workloadError(w, workload.ErrConflict)
			return
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-SHA256", file.SHA256)
	controller := http.NewResponseController(w)
	offset := int64(0)
	digest := sha256.New()
	for {
		if _, err = s.Identity.Authenticate(r.Context(), s.readCookie(r, "")); err != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if len(data) > 0 {
			_, _ = digest.Write(data)
			if offset+int64(len(data)) == file.Size && hex.EncodeToString(digest.Sum(nil)) != file.SHA256 {
				return
			}
			if _, err = w.Write(data); err != nil {
				return
			}
			offset += int64(len(data))
		}
		if offset == file.Size {
			return
		}
		data, err = s.Workloads.Download(r.Context(), id, offset)
		if err != nil {
			return
		}
	}
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.Jobs(r.Context())
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	if !actor(r).Allows("files") {
		fail(w, 403, "POLICY_DENIED", "Video jobs require both jobs and files access.")
		return
	}
	var body struct {
		InputID    string `json:"inputId"`
		Preset     string `json:"preset"`
		RetryGroup string `json:"retryGroup"`
	}
	if !decode(w, r, &body) {
		return
	}
	job, err := s.Workloads.CreateJob(r.Context(), actor(r).Device.ID, r.Header.Get("Idempotency-Key"), body.InputID, body.Preset, body.RetryGroup)
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 202, job)
}
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AttemptID string `json:"attemptId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Workloads.CancelJob(r.Context(), actor(r).Device.ID, r.PathValue("id"), body.AttemptID); err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 202, map[string]bool{"cancellationRequested": true})
}

func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.Conversations(r.Context())
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	item, err := s.Workloads.CreateConversation(r.Context(), body.Title)
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.History(r.Context(), r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) createGeneration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversationId"`
		Prompt         string `json:"prompt"`
	}
	if !decode(w, r, &body) {
		return
	}
	item, err := s.Workloads.CreateGeneration(r.Context(), actor(r).Device.ID, r.Header.Get("Idempotency-Key"), body.ConversationID, body.Prompt)
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 202, item)
}
func (s *Server) generation(w http.ResponseWriter, r *http.Request) {
	item, err := s.Workloads.Generation(r.Context(), r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) cancelGeneration(w http.ResponseWriter, r *http.Request) {
	if err := s.Workloads.CancelGeneration(r.Context(), r.PathValue("id")); err != nil {
		workloadError(w, err)
		return
	}
	writeJSON(w, 202, map[string]bool{"cancellationRequested": true})
}
func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	if !identity.ValidEmptyApprovalBody(body) {
		s.authError(w, identity.ErrDenied)
		return
	}
	grant := r.Header.Get("X-Action-Approval")
	if grant == "" {
		fail(w, 403, "APPROVAL_REQUIRED", "Approve deleting this conversation with your passkey.")
		return
	}
	id, key := r.PathValue("id"), r.Header.Get("Idempotency-Key")
	resources, err := s.conversationDeleteResources(id, key)
	if err != nil {
		workloadError(w, err)
		return
	}
	var op workload.Operation
	err = s.Identity.ConsumeApproval(r.Context(), actor(r), grant, "ai.delete", resources, body, s.config.PolicyGeneration, func(tx *sql.Tx) error {
		var err error
		op, err = s.Workloads.RequestConversationDeletionInTransaction(tx, actor(r).Device.ID, key, id)
		return err
	})
	if err != nil {
		if errors.Is(err, identity.ErrDenied) {
			s.authError(w, err)
		} else {
			workloadError(w, err)
		}
		return
	}
	writeJSON(w, 202, op)
}
func (s *Server) exportConversation(w http.ResponseWriter, r *http.Request) {
	items, err := s.Workloads.History(r.Context(), r.PathValue("id"))
	if err != nil {
		workloadError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=homenode-conversation.json")
	writeJSON(w, 200, map[string]any{"schema": 1, "generations": items})
}
