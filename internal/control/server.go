// Package control exposes first-party management operations. This process has
// no libvirt, QMP, shell or root authority.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

type Config struct {
	// BackupRepositoryID is supplied by trusted host provisioning, never a request.
	BackupRepositoryID string
	BackupExecution    *BackupExecutionConfig
	Runtime            workload.Backend
	PolicyGeneration   int64
	Origin             string
	Development        bool
	UI                 fs.FS
	Report             func() hostcheck.Report
}
type Server struct {
	backupTasks *backupTasks
	Workloads   *workload.Service
	config      Config
	Identity    *identity.Service
	Store       *state.Store
	mux         *http.ServeMux
	host        string
	cookie      string
	slots       chan struct{}
	rateMu      sync.Mutex
	authCount   int
	authWindow  time.Time
}
type sessionKey struct{}

func New(store *state.Store, config Config) (*Server, error) {
	if err := validateBackupExecution(config); err != nil {
		return nil, err
	}
	if config.BackupExecution != nil {
		execution := *config.BackupExecution
		config.BackupExecution = &execution
	}
	if config.BackupRepositoryID != "" {
		body, err := json.Marshal(map[string]string{"repositoryId": config.BackupRepositoryID})
		if err != nil {
			return nil, err
		}
		if _, err = identity.BackupApprovalResources(body, config.BackupRepositoryID, "configuration-validation"); err != nil {
			return nil, errors.New("invalid registered backup repository identity")
		}
		if config.Development {
			return nil, errors.New("development mode cannot configure external backups")
		}
	}
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" {
		return nil, errors.New("origin must be a scheme and host without a path")
	}
	if config.Development {
		if origin.Scheme != "http" || origin.Hostname() != "localhost" {
			return nil, errors.New("development origin must use http://localhost")
		}
	} else if origin.Scheme != "https" {
		return nil, errors.New("production requires an HTTPS origin")
	}
	auth, err := identity.New(store, config.Origin)
	if err != nil {
		return nil, err
	}
	// Changing RP identity requires an explicit recovery process, not a flag.
	_, err = store.DB.Exec("INSERT OR IGNORE INTO settings(key,value) VALUES('origin',?)", config.Origin)
	if err != nil {
		return nil, err
	}
	var pinned string
	if err = store.DB.QueryRow("SELECT value FROM settings WHERE key='origin'").Scan(&pinned); err != nil {
		return nil, err
	}
	if pinned != config.Origin {
		return nil, errors.New("origin differs from initialized identity; use its original origin or replacement-host recovery")
	}
	s := &Server{backupTasks: newBackupTasks(), config: config, Identity: auth, Store: store, mux: http.NewServeMux(), host: origin.Host, cookie: "__Host-homenode", slots: make(chan struct{}, 64)}
	if config.Development {
		s.cookie = "homenode-dev"
	}
	s.Workloads = workload.New(store, config.Runtime, config.PolicyGeneration)
	s.routes()
	s.workloadRoutes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/setup", s.setupStatus)
	s.approvalRoutes()
	s.mux.HandleFunc("POST /api/v1/auth/register/begin", s.registerBegin)
	s.mux.HandleFunc("POST /api/v1/auth/register/finish", s.registerFinish)
	s.mux.HandleFunc("POST /api/v1/auth/login/begin", s.loginBegin)
	s.mux.HandleFunc("POST /api/v1/auth/login/finish", s.loginFinish)
	s.mux.HandleFunc("POST /api/v1/auth/recover", s.recover)
	s.mux.Handle("GET /api/v1/session", s.require("", false, http.HandlerFunc(s.session)))
	s.mux.Handle("POST /api/v1/logout", s.require("", false, http.HandlerFunc(s.logout)))
	s.mux.Handle("GET /api/v1/devices", s.require("admin", false, http.HandlerFunc(s.devices)))
	s.mux.Handle("POST /api/v1/devices/pair", s.require("admin", false, http.HandlerFunc(s.pair)))
	s.mux.Handle("POST /api/v1/devices/{id}/revoke", s.require("admin", false, http.HandlerFunc(s.revoke)))
	s.mux.Handle("GET /api/v1/host/report", s.require("", false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.config.Report()) })))
	s.mux.Handle("GET /api/v1/events", s.require("", false, http.HandlerFunc(s.events)))
	s.mux.Handle("GET /api/v1/diagnostics", s.require("admin", false, http.HandlerFunc(s.diagnostics)))
	s.mux.Handle("GET /api/v1/backups/configuration", s.require("admin", false, http.HandlerFunc(s.backupConfiguration)))
	s.mux.Handle("POST /api/v1/backups", s.require("admin", false, http.HandlerFunc(s.backupCreate)))
	s.mux.Handle("GET /api/v1/backups/outcomes", s.require("admin", false, http.HandlerFunc(s.backupOutcomes)))
	s.mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "alive"}) })
	s.mux.HandleFunc("GET /", s.static)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), publickey-credentials-get=(self), publickey-credentials-create=(self)")
	if !s.config.Development {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
	if r.Host != s.host || r.URL.IsAbs() {
		fail(w, 403, "ORIGIN_DENIED", "Use the configured HomeNode address.")
		return
	}
	if !s.config.Development && r.TLS == nil {
		fail(w, 403, "TLS_REQUIRED", "A trusted HTTPS connection is required.")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.config.Origin {
		fail(w, 403, "ORIGIN_DENIED", "Cross-origin access is denied.")
		return
	}
	if fetch := r.Header.Get("Sec-Fetch-Site"); fetch == "cross-site" {
		fail(w, 403, "ORIGIN_DENIED", "Cross-site access is denied.")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.Header.Get("Origin") != s.config.Origin {
			fail(w, 403, "ORIGIN_DENIED", "The request needs its original browser origin.")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		expectedMedia := "application/json"
		if r.Method == "POST" && r.URL.Path == "/api/v1/backups" {
			expectedMedia = backupCredentialMediaType
		}
		if err != nil || media != expectedMedia {
			fail(w, 415, "INVALID_CONTENT_TYPE", "Send the required request content type.")
			return
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		fail(w, 503, "BUSY", "Retry after current requests finish.")
		return
	}
	limit := int64(64 << 10)
	if strings.HasPrefix(r.URL.Path, "/api/v1/transfers/") && strings.HasSuffix(r.URL.Path, "/chunks") {
		limit = 512 << 10
	}
	if r.Method == "POST" && r.URL.Path == "/api/v1/backups" {
		limit = maxBackupCredentialRequest
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if (strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || strings.HasSuffix(r.URL.Path, "/approval")) && !s.allowAuth() {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "RATE_LIMITED", "Wait a minute before another authentication attempt.")
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) allowAuth() bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	now := time.Now()
	if now.Sub(s.authWindow) >= time.Minute {
		s.authWindow = now
		s.authCount = 0
	}
	s.authCount++
	return s.authCount <= 30
}
func (s *Server) readCookie(r *http.Request, suffix string) string {
	c, err := r.Cookie(s.cookie + suffix)
	if err != nil {
		return ""
	}
	return c.Value
}
func (s *Server) setCookie(w http.ResponseWriter, suffix, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: s.cookie + suffix, Value: value, Path: "/", Secure: !s.config.Development, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func (s *Server) require(capability string, fresh bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := s.Identity.Authenticate(r.Context(), s.readCookie(r, ""))
		if err != nil {
			fail(w, 401, "AUTH_REQUIRED", "Sign in with your passkey.")
			return
		}
		if capability != "" && !session.Allows(capability) {
			fail(w, 403, "POLICY_DENIED", "This device does not have permission for that action.")
			return
		}
		if fresh && !session.Fresh() {
			fail(w, 403, "REAUTH_REQUIRED", "Verify your passkey again before this action.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, session)))
	})
}
func actor(r *http.Request) identity.Session {
	return r.Context().Value(sessionKey{}).(identity.Session)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		fail(w, 400, "INVALID_REQUEST", "The request is malformed or too large.")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "INVALID_REQUEST", "Send a single JSON object.")
		return false
	}
	return true
}
func (s *Server) authError(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrConflict) {
		fail(w, 409, "IDENTITY_CONFLICT", "Keep another administrator enrolled, or restart enrollment after an identity change.")
		return
	}
	fail(w, 403, "AUTH_DENIED", "The code, passkey, or authorization is invalid or expired.")
}
func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	claimed, _, err := s.Identity.Status(r.Context())
	if err != nil {
		fail(w, 500, "STATE_UNAVAILABLE", "Identity storage is unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"claimed": claimed, "development": s.config.Development})
}
func (s *Server) registerBegin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decode(w, r, &input) {
		return
	}
	options, token, err := s.Identity.BeginRegistration(r.Context(), input.Code, input.Name)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, "-challenge", token, 300)
	writeJSON(w, 200, options)
}
func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	token, codes, err := s.Identity.FinishRegistration(r.Context(), s.readCookie(r, "-challenge"), r)
	s.setCookie(w, "-challenge", "", -1)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, "", token, 12*3600)
	writeJSON(w, 201, map[string]any{"recoveryCodes": codes})
}
func (s *Server) loginBegin(w http.ResponseWriter, r *http.Request) {
	options, token, err := s.Identity.BeginLogin(r.Context())
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, "-challenge", token, 300)
	writeJSON(w, 200, options)
}
func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	token, err := s.Identity.FinishLogin(r.Context(), s.readCookie(r, "-challenge"), s.readCookie(r, ""), r)
	s.setCookie(w, "-challenge", "", -1)
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setCookie(w, "", token, 12*3600)
	writeJSON(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) recover(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	token, err := s.Identity.Recovery(r.Context(), input.Code)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"code": token})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, actor(r)) }
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Identity.Logout(r.Context(), actor(r)); err != nil {
		fail(w, 500, "STATE_UNAVAILABLE", "Could not close this session.")
		return
	}
	s.setCookie(w, "", "", -1)
	writeJSON(w, 200, map[string]bool{"authenticated": false})
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.Identity.Devices(r.Context())
	if err != nil {
		fail(w, 500, "STATE_UNAVAILABLE", "Could not read devices.")
		return
	}
	writeJSON(w, 200, devices)
}
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	if r.Header.Get("X-Action-Approval") == "" {
		fail(w, 403, "APPROVAL_REQUIRED", "Approve this device invitation with your passkey.")
		return
	}
	code, err := s.Identity.PairApproved(r.Context(), actor(r), r.Header.Get("X-Action-Approval"), body, s.config.PolicyGeneration)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"code": code, "expiresIn": 300})
}
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	body, ok := approvalBody(w, r)
	if !ok {
		return
	}
	if r.Header.Get("X-Action-Approval") == "" {
		fail(w, 403, "APPROVAL_REQUIRED", "Approve revoking this device with your passkey.")
		return
	}
	if err := s.Identity.RevokeApproved(r.Context(), actor(r), r.Header.Get("X-Action-Approval"), r.PathValue("id"), body, s.config.PolicyGeneration); err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"revoked": true})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = r.URL.Query().Get("after")
	}
	var after int64
	var err error
	if cursor != "" {
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 0 {
			fail(w, 400, "INVALID_CURSOR", "Use a valid event cursor.")
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	// Reconnect periodically. Every iteration rechecks revocation; no cached
	// session can keep a stream alive after its device is revoked.
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		current, err := s.Identity.Authenticate(r.Context(), s.readCookie(r, ""))
		if err != nil {
			return
		}
		rows, err := s.Store.DB.QueryContext(r.Context(), "SELECT id,kind,object_id,payload,created_at FROM events WHERE id>? AND (actor=? OR ?=1) ORDER BY id LIMIT 100", after, current.Device.ID, current.Allows("admin"))
		if err != nil {
			return
		}
		type event struct {
			ID        int64           `json:"id"`
			Kind      string          `json:"kind"`
			ObjectID  string          `json:"objectId"`
			Payload   json.RawMessage `json:"payload"`
			CreatedAt int64           `json:"createdAt"`
		}
		batch := []event{}
		for rows.Next() {
			var e event
			var payload string
			if err = rows.Scan(&e.ID, &e.Kind, &e.ObjectID, &payload, &e.CreatedAt); err != nil {
				break
			}
			e.Payload = json.RawMessage(payload)
			batch = append(batch, e)
		}
		rowErr := rows.Err()
		_ = rows.Close()
		if err != nil || rowErr != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		for _, e := range batch {
			data, _ := json.Marshal(e)
			if _, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.ID, data); err != nil {
				return
			}
			after = e.ID
		}
		if _, err = io.WriteString(w, ": heartbeat\n\n"); err != nil {
			return
		}
		if err = controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	report := s.config.Report()
	report.Host.DiskProbePath = "[redacted]"
	for i := range report.Checks {
		if report.Checks[i].ID == "disk" {
			report.Checks[i].Detail = "Storage measurement available; host path omitted."
		}
	}
	writeJSON(w, 200, map[string]any{"schema": 1, "host": report, "identity": "omitted", "content": "omitted", "sessions": "omitted"})
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if s.config.UI == nil || !fs.ValidPath(name) || (name != "index.html" && !strings.HasPrefix(name, "assets/")) {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.config.UI, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(data) > 8<<20 {
		http.Error(w, "asset too large", 500)
		return
	}
	content := mime.TypeByExtension(path.Ext(name))
	if content == "" {
		content = "application/octet-stream"
	}
	w.Header().Set("Content-Type", content)
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.WriteHeader(200)
	if r.Method != "HEAD" {
		_, _ = w.Write(data)
	}
}
