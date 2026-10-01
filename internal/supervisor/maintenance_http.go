package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// MaintenanceHandler exposes only acquisition/release to a separately configured
// backup peer. Mount it on a protected local socket with PeerContext; no HTTP
// header or controller request can supply the peer identity. Production socket
// installation and disk descriptor transport remain separate integration work.
func (m *Manager) MaintenanceHandler(backupUID uint32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := RequestPeerUID(r)
		if !ok || backupUID == 0 || backupUID == m.Policy.ControllerUID || backupUID == m.Policy.TransferUID || uid != backupUID {
			http.Error(w, "peer denied", http.StatusForbidden)
			return
		}
		if r.Method != "POST" || (r.URL.Path != "/v1/maintenance/begin" && r.URL.Path != "/v1/maintenance/end") {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512))
		if err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		var request struct {
			Version int    `json:"version"`
			JobID   string `json:"jobId"`
			Token   string `json:"token"`
		}
		if !uniqueMaintenanceObject(data) {
			http.Error(w, "invalid request", 400)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Version != 1 {
			http.Error(w, "invalid request", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		token := ""
		if r.URL.Path == "/v1/maintenance/begin" {
			if request.JobID == "" || request.Token != "" {
				http.Error(w, "invalid request", 400)
				return
			}
			token, err = m.BeginRuntimeMaintenanceForJob(ctx, request.JobID)
		} else {
			if request.Token == "" || request.JobID != "" {
				http.Error(w, "invalid request", 400)
				return
			}
			err = m.EndRuntimeMaintenance(ctx, request.Token)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "MAINTENANCE_BLOCKED"})
			return
		}
		if token != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "token": token})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": 1})
		}
	})
}

// Only scalar typed fields are accepted by the subsequent decoder. Check decoded
// keys before struct decoding to reject duplicate fields and escaped aliases.
func uniqueMaintenanceObject(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		return false
	}
	return d.Decode(new(any)) == io.EOF
}
