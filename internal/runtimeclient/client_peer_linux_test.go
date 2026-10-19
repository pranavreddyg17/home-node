//go:build linux

package runtimeclient

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkloadClientAuthenticatesListenerBeforeSendingOperation(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "runtime.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	uid := uint32(os.Geteuid())
	foreign := uid + 1
	for _, expected := range []uint32{foreign, 1 << 31} {
		client := authenticatedSocketClient(socket, expected)
		var result map[string]bool
		err := call(ctx, client, "/v1/runtime", map[string]string{"operation": "must remain private"}, &result)
		client.Transport.(*http.Transport).CloseIdleConnections()
		if err == nil || requests.Load() != 0 {
			t.Fatal("foreign listener received workload authority", expected, err, requests.Load())
		}
	}
	client := authenticatedSocketClient(socket, uid)
	defer client.Transport.(*http.Transport).CloseIdleConnections()
	var result map[string]bool
	if err := call(ctx, client, "/v1/runtime", map[string]string{"operation": "fixture"}, &result); err != nil || !result["accepted"] || requests.Load() != 1 {
		t.Fatal("matching kernel listener refused", err, result, requests.Load())
	}
	// The controller must never treat the root supervisor as its transfer role.
	invalid := New(socket, socket, 0)
	defer invalid.transfer.Transport.(*http.Transport).CloseIdleConnections()
	if err := call(ctx, invalid.transfer, "/v1/guest", map[string]string{"operation": "must remain private"}, &result); err == nil || requests.Load() != 1 {
		t.Fatal("zero transfer identity enabled a guest request", err, requests.Load())
	}
	if uid != 0 {
		defer invalid.runtime.Transport.(*http.Transport).CloseIdleConnections()
		if err := call(ctx, invalid.runtime, "/v1/runtime", map[string]string{"operation": "must remain private"}, &result); err == nil || requests.Load() != 1 {
			t.Fatal("nonroot listener impersonated supervisor", err, requests.Load())
		}
	}
}
