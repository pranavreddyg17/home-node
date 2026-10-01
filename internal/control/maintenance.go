package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type maintenanceRequest struct {
	Version  int    `json:"version"`
	Token    string `json:"token"`
	JobID    string `json:"jobId"`
	DeviceID string `json:"deviceId"`
}

func decodeMaintenanceRequest(data []byte) (request maintenanceRequest, ok bool) {
	if len(data) > 512 || !utf8.Valid(data) {
		return request, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return request, false
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return request, false
		}
		name, ok := key.(string)
		if !ok {
			return request, false
		}
		if _, exists := fields[name]; exists {
			return request, false
		}
		if name != "version" && name != "token" && name != "jobId" && name != "deviceId" {
			return request, false
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return request, false
		}
		fields[name] = raw
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || decoder.Decode(new(any)) != io.EOF || len(fields) != 4 {
		return request, false
	}
	if json.Unmarshal(data, &request) != nil || request.Version != 1 || !guestproto.ValidID(request.Token) || !guestproto.ValidID(request.JobID) || !guestproto.ValidID(request.DeviceID) {
		return request, false
	}
	return request, true
}

// MaintenanceHandler belongs only on a protected local socket using
// supervisor.PeerContext. It accepts an existing owned maintenance job and can
// only drain/restore its apps; it cannot acquire admission or mutate runtimes.
func (s *Server) MaintenanceHandler(controllerUID, backupUID uint32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := supervisor.RequestPeerUID(r)
		if !ok || controllerUID == 0 || backupUID == 0 || backupUID == controllerUID || uid != backupUID {
			http.Error(w, "peer denied", 403)
			return
		}
		if r.Method != "POST" || r.URL.RawQuery != "" || (r.URL.Path != "/v1/maintenance/drain" && r.URL.Path != "/v1/maintenance/restore" && r.URL.Path != "/v1/maintenance/snapshot" && r.URL.Path != "/v1/maintenance/verify-stage") {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512))
		if err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		request, ok := decodeMaintenanceRequest(data)
		if !ok {
			http.Error(w, "invalid request", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		job, err := s.Store.InspectMaintenanceJob(ctx, request.Token)
		if err != nil || job.ID != request.JobID || job.Device != request.DeviceID {
			http.Error(w, "maintenance blocked", 409)
			return
		}
		if r.URL.Path == "/v1/maintenance/verify-stage" {
			inventory, err := s.Store.InspectMaintenance(ctx, request.Token)
			if err != nil || job.Phase != "staging" || job.RootToken == "" || inventory != (state.MaintenanceInventory{}) {
				http.Error(w, "maintenance blocked", 409)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{"version": 1})
			return
		}
		if r.URL.Path == "/v1/maintenance/snapshot" {
			if job.Phase != "staging" || job.RootToken == "" {
				http.Error(w, "maintenance blocked", 409)
				return
			}
			stream := &snapshotResponse{ResponseWriter: w}
			if err = s.writeMaintenanceSnapshot(ctx, request.Token, stream); err != nil && !stream.started {
				w.Header().Del("Content-Length")
				http.Error(w, "maintenance snapshot blocked", 409)
			}

			return
		}
		if r.URL.Path == "/v1/maintenance/drain" {
			if job.Phase != "draining" {
				http.Error(w, "maintenance blocked", 409)
				return
			}
			err = s.Workloads.DrainMaintenance(ctx, request.Token, request.DeviceID)
		} else {
			if job.Phase != "restoring" || job.RootToken != "" {
				http.Error(w, "maintenance blocked", 409)
				return
			}
			err = s.Workloads.RestoreMaintenanceApps(ctx, request.Token, request.DeviceID)
		}
		if err != nil {
			http.Error(w, "maintenance blocked", 409)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"version": 1})
	})
}
