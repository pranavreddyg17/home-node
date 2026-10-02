package control

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestBackupHTTPRequiresAdminOriginMediaAndBoundedFrame(t *testing.T) {
	s := testServer(t)
	origin := "http://localhost:8787"
	admin := seedSession(t, s, `["admin"]`)
	send := func(cookie, requestOrigin, media, path string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", origin+path, bytes.NewReader(body))
		r.URL.Scheme = ""
		r.URL.Host = ""
		r.Header.Set("Origin", requestOrigin)
		r.Header.Set("Content-Type", media)
		r.Header.Set("Idempotency-Key", "backup-request-1234567890")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: s.cookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if got := send("", origin, backupCredentialMediaType, "/api/v1/backups", nil); got.Code != 401 {
		t.Fatal(got.Code)
	}
	if got := send(admin, origin, backupCredentialMediaType, "/api/v1/backups", nil); got.Code != 503 {
		t.Fatal(got.Code)
	}
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.Runtime = fileBackend{}
	s.config.BackupExecution = &BackupExecutionConfig{Release: "0.1.0", CatalogVersion: 1, Launch: func(context.Context, backup.Launch, *os.File) error { t.Fatal("unexpected launch"); return nil }, Cleanup: refusedBackupCleanup(t)}
	for _, tc := range []struct {
		origin, media, path string
		body                []byte
		status              int
	}{
		{"https://foreign.example", backupCredentialMediaType, "/api/v1/backups", nil, 403},
		{origin, "application/json", "/api/v1/backups", nil, 415},
		{origin, backupCredentialMediaType, "/api/v1/backups", []byte("malformed"), 400},
		{origin, backupCredentialMediaType, "/api/v1/backups", bytes.Repeat([]byte("x"), maxBackupCredentialRequest+1), 400},
		{origin, backupCredentialMediaType, "/api/v1/backups?password=secret", nil, 400},
		{origin, backupCredentialMediaType, "/api/v1/backups/approval", nil, 415},
	} {
		got := send(admin, tc.origin, tc.media, tc.path, tc.body)
		if got.Code != tc.status || strings.Contains(got.Body.String(), "secret") {
			t.Fatal(got.Code, got.Body.String())
		}
		if got.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credential route cached")
		}
	}
	if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
		t.Fatal("refused request closed admission", err)
	}
}

func TestBackupExecutionConfigurationRequiresInstalledMetadata(t *testing.T) {
	s := testServer(t)
	launch := func(context.Context, backup.Launch, *os.File) error { return nil }
	for _, scenario := range []string{"development", "no-repository", "no-runtime", "no-launch", "no-cleanup", "release", "catalog"} {
		config := s.config
		config.Development = false
		config.BackupRepositoryID = strings.Repeat("a", 64)
		config.Runtime = fileBackend{}
		config.BackupExecution = &BackupExecutionConfig{Release: "0.1.0", CatalogVersion: 1, Launch: launch, Cleanup: refusedBackupCleanup(t)}
		switch scenario {
		case "development":
			config.Development = true
		case "no-repository":
			config.BackupRepositoryID = ""
		case "no-runtime":
			config.Runtime = nil
		case "no-launch":
			config.BackupExecution.Launch = nil
		case "no-cleanup":
			config.BackupExecution.Cleanup = nil
		case "release":
			config.BackupExecution.Release = "invalid"
		case "catalog":
			config.BackupExecution.CatalogVersion = 0
		}
		if err := validateBackupExecution(config); err == nil {
			t.Fatal("invalid execution accepted", scenario)
		}
	}
}

func TestBackupConfigurationEndpointOnlyExposesEnabledRegisteredIdentity(t *testing.T) {
	s := testServer(t)
	origin := "http://localhost:8787"
	path := origin + "/api/v1/backups/configuration"
	if got := request(s, "GET", path, "", "", ""); got.Code != 401 {
		t.Fatal(got.Code)
	}
	token := seedSession(t, s, `["admin"]`)
	got := request(s, "GET", path, "", token, "")
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"enabled":false`) || !strings.Contains(got.Body.String(), `"repositoryId":""`) {
		t.Fatal(got.Code, got.Body.String())
	}
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	s.config.BackupExecution = &BackupExecutionConfig{}
	got = request(s, "GET", path, "", token, "")
	if got.Code != 200 || !strings.Contains(got.Body.String(), s.config.BackupRepositoryID) || strings.Contains(got.Body.String(), "release") || strings.Contains(got.Body.String(), "socket") {
		t.Fatal(got.Code, got.Body.String())
	}
	if !strings.Contains(got.Body.String(), `"availability":"available"`) {
		t.Fatal("idle configured backup unavailable", got.Body.String())
	}
	s.backupTasks.mu.Lock()
	s.backupTasks.active = true
	s.backupTasks.mu.Unlock()
	paused := request(s, "GET", path, "", token, "")
	if paused.Code != 200 || !strings.Contains(paused.Body.String(), `"availability":"paused"`) {
		t.Fatal("active work offered backup", paused.Body.String())
	}
	s.backupTasks.mu.Lock()
	s.backupTasks.active = false
	s.backupTasks.mu.Unlock()
	maintenance, err := s.Store.BeginMaintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	paused = request(s, "GET", path, "", token, "")
	if paused.Code != 200 || !strings.Contains(paused.Body.String(), `"availability":"paused"`) || strings.Contains(paused.Body.String(), maintenance) {
		t.Fatal("maintenance availability leaked or reopened", paused.Body.String())
	}
	if got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("configuration cached")
	}
}
