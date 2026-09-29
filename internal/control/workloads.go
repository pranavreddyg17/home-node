package control

import (
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/workload"
)

func (s *Server) workloadRoutes() {
	s.mux.Handle("GET /api/v1/apps", s.require("", false, http.HandlerFunc(s.apps)))
	s.mux.Handle("POST /api/v1/apps/{workload}/actions", s.require("admin", true, http.HandlerFunc(s.appAction)))
	s.mux.Handle("GET /api/v1/operations/{id}", s.require("", false, http.HandlerFunc(s.operation)))
	s.mux.Handle("POST /api/v1/transfers", s.require("files", false, http.HandlerFunc(s.createTransfer)))
	s.mux.Handle("GET /api/v1/transfers/{id}", s.require("files", false, http.HandlerFunc(s.transfer)))
	s.mux.Handle("POST /api/v1/transfers/{id}/chunks", s.require("files", false, http.HandlerFunc(s.upload)))
	s.mux.Handle("POST /api/v1/transfers/{id}/finalize", s.require("files", false, http.HandlerFunc(s.finalize)))
	s.mux.Handle("GET /api/v1/files", s.require("files", false, http.HandlerFunc(s.files)))
	s.mux.Handle("POST /api/v1/files/{id}/actions", s.require("files", false, http.HandlerFunc(s.changeFile)))
	s.mux.Handle("GET /api/v1/files/{id}/download", s.require("files", false, http.HandlerFunc(s.download)))
}
func workloadError(w http.ResponseWriter, err error) {
	switch {
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
	var body struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &body) {
		return
	}
	op, err := s.Workloads.AppAction(r.Context(), actor(r).Device.ID, r.Header.Get("Idempotency-Key"), r.PathValue("workload"), body.Action)
	if err != nil {
		workloadError(w, err)
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
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-SHA256", file.SHA256)
	controller := http.NewResponseController(w)
	offset := int64(0)
	for {
		if _, err = s.Identity.Authenticate(r.Context(), s.readCookie(r, "")); err != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if len(data) > 0 {
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
