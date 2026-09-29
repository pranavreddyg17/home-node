package supervisor

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

type peerKey struct{}

func PeerContext(ctx context.Context, connection net.Conn) context.Context {
	unix, ok := connection.(*net.UnixConn)
	if !ok {
		return ctx
	}
	uid, err := PeerUID(unix)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, peerKey{}, uid)
}
func (m *Manager) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := r.Context().Value(peerKey{}).(uint32)
		if !ok || uid != m.Policy.ControllerUID && uid != m.Policy.TransferUID {
			http.Error(w, "peer denied", 403)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/v1/runtime" {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request Request
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid request", 400)
			return
		}
		if uid == m.Policy.TransferUID && request.Action != "inspect" {
			http.Error(w, "operation denied", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		instance, err := m.Apply(ctx, request)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "RUNTIME_BLOCKED"})
			return
		}
		_ = json.NewEncoder(w).Encode(instance)
	})
}

func RequestPeerUID(r *http.Request) (uint32, bool) {
	uid, ok := r.Context().Value(peerKey{}).(uint32)
	return uid, ok
}
