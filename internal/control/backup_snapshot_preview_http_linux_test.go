//go:build linux

package control

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSnapshotPreviewHTTPSealedCredentialAndPostDispatchAuthorization(t *testing.T) {
	for _, scenario := range []string{"valid", "revoked", "worker-error", "foreign-snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			s := testServer(t)
			token := seedBackupSession(t, s)
			if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
				t.Fatal(err)
			}
			repository := state.Hash("repository")
			cursor := state.Hash("cursor")
			candidate := state.Hash("candidate")
			s.config.BackupRepositoryID = repository
			var owned *os.File
			var dispatch backup.SnapshotPreviewRequest
			calls := 0
			s.config.BackupExecution = &BackupExecutionConfig{SnapshotPreview: func(ctx context.Context, request backup.SnapshotPreviewRequest, credential *os.File) (backup.SnapshotPreview, error) {
				calls++
				owned, dispatch = credential, request
				if request.SnapshotID != cursor || request.RequestID == "" || request.DeviceID == "" {
					t.Fatal("selector binding lost", request)
				}
				if err := s.verifySnapshotPreviewRequest(ctx, request); err != nil {
					t.Fatal("pending authority missing", err)
				}
				password, err := backup.ReadRepositoryPassword(ctx, credential)
				if err != nil || string(password) != "fixture-only-selector-secret" {
					t.Fatal("sealed credential lost", err)
				}
				clear(password)
				if scenario == "revoked" {
					if _, err := s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", request.DeviceID); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "worker-error" {
					return backup.SnapshotPreview{}, errors.New("fixture-only-selector-secret")
				}
				page := backup.SnapshotPreview{SnapshotID: cursor, CreatedAt: time.Now().Add(-time.Minute), Release: "0.1.0", CatalogVersion: 1, Validation: "metadata-compatible", Files: []backup.SnapshotPreviewFile{{Workload: "management", Bytes: 1024}}}
				if scenario == "foreign-snapshot" {
					page.SnapshotID = candidate
				}
				return page, nil
			}}
			metadata := []byte(`{"repositoryId":"` + repository + `","snapshotId":"` + cursor + `"}`)
			frame := make([]byte, 4)
			binary.BigEndian.PutUint32(frame, uint32(len(metadata)))
			frame = append(frame, metadata...)
			frame = append(frame, []byte("fixture-only-selector-secret")...)
			req := httptest.NewRequest("POST", "http://localhost:8787/api/v1/backups/preview", bytes.NewReader(frame))
			req.URL.Scheme = ""
			req.URL.Host = ""
			req.Header.Set("Origin", s.config.Origin)
			req.Header.Set("Content-Type", backupCredentialMediaType)
			req.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, req)
			if calls != 1 || owned == nil {
				t.Fatal("snapshot delivery not executed", response.Code, response.Body.String())
			}
			if _, err := owned.Stat(); err == nil {
				t.Fatal("HTTP handler retained credential")
			}
			if err := s.verifySnapshotPreviewRequest(context.Background(), dispatch); err == nil {
				t.Fatal("HTTP completion retained worker authority")
			}
			if strings.Contains(response.Body.String(), "fixture-only-selector-secret") {
				t.Fatal("credential reflected")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("selector response cached")
			}
			if scenario == "valid" {
				var page backup.SnapshotPreview
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.SnapshotID != cursor || page.Validation != "metadata-compatible" || len(page.Files) != 1 {
					t.Fatal("candidate response lost", response.Code, response.Body.String())
				}
			} else if response.Code == 200 || strings.Contains(response.Body.String(), candidate) {
				t.Fatal("unqualified candidates disclosed", response.Code, response.Body.String())
			}
			if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
				t.Fatal("selector acquired maintenance", err)
			}
		})
	}
}
