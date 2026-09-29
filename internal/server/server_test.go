package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
)

func TestHostHeaderAndReadiness(t *testing.T) {
	handler := New(func() hostcheck.Report {
		return hostcheck.Report{GeneratedAt: time.Unix(0, 0), ExecutionEnabled: false}
	})
	for _, tc := range []struct {
		host, path string
		status     int
	}{
		{"127.0.0.1:8787", "/api/v1/healthz", http.StatusOK},
		{"localhost:8787", "/api/v1/host/report", http.StatusOK},
		{"evil.example:8787", "/api/v1/host/report", http.StatusForbidden},
		{"127.0.0.1:8787.evil.example", "/api/v1/host/report", http.StatusForbidden},
		{"127.0.0.1:8787", "/api/v1/ready", http.StatusServiceUnavailable},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Host = tc.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Errorf("%s %s: status %d, want %d", tc.host, tc.path, response.Code, tc.status)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Error("diagnostic response may be cached")
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/host/report", nil)
	req.Host = "localhost:8787"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	var report hostcheck.Report
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.ExecutionEnabled {
		t.Fatalf("report must remain truthful: %v, %+v", err, report)
	}
}
