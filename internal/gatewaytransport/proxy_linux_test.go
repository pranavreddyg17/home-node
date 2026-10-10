//go:build linux

package gatewaytransport

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Same-UID temporary peer fixture checks kernel pinning and header mediation.
// Separate installed identities and service confinement need native root tests.
func TestUnixControllerPeerAndHeaderMediation(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("unprivileged Linux kernel peer fixture")
	}
	socket := filepath.Join(t.TempDir(), "controller.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0660); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	handler, err := ControllerHandler(uint32(os.Geteuid()), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !TrustedHTTPS(r) || r.RemoteAddr != "100.64.0.2:0" || r.Host != "home.example.test:8787" || r.Header.Get("Origin") != "https://home.example.test:8787" || r.Header.Get("Cookie") != "session=fixture" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get(clientAddressHeader) != "" || r.Header.Get(clientTLSHeader) != "" {
			t.Error("proxy boundary changed or trusted public assertions")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	invoke := func(uid uint32) int {
		t.Helper()
		proxy, close, err := NewProxy(Config{Origin: "https://home.example.test:8787", Socket: socket, ControllerUID: uid, AccessGID: uint32(os.Getegid())})
		if err != nil {
			t.Fatal(err)
		}
		defer close()
		request := httptest.NewRequest("GET", "/api/v1/healthz", nil)
		request.Host = "home.example.test:8787"
		request.RemoteAddr = "100.64.0.2:12345"
		request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
		request.Header.Set("Cookie", "session=fixture")
		request.Header.Set("Origin", "https://home.example.test:8787")
		request.Header.Set(clientAddressHeader, "100.64.0.99")
		request.Header.Set(clientTLSHeader, "unsafe")
		request.Header.Set("Forwarded", "for=100.64.0.99;proto=http")
		request.Header.Set("X-Forwarded-For", "100.64.0.99")
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, request)
		return response.Code
	}
	if status := invoke(uint32(os.Geteuid()) + 1); status != 503 || requests.Load() != 0 {
		t.Fatal("foreign controller owner accepted", status)
	}
	if status := invoke(uint32(os.Geteuid())); status != 200 || requests.Load() != 1 {
		t.Fatal("qualified peer request failed", status)
	}
}
