//go:build linux

package control

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/gatewaytransport"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// This temporary same-UID fixture proves the controller's transport guard;
// installed distinct identities and systemd confinement require separate proof.
func TestProductionControllerThroughAuthenticatedUnixGateway(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unprivileged kernel peer fixture")
	}
	controller := testServer(t)
	controller.config.Development = false
	controller.config.Origin = "https://node.example"
	controller.host = "node.example"
	handler, err := gatewaytransport.ControllerHandler(uint32(os.Geteuid()), controller)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, qualified := range []bool{false, true} {
		request, err := http.NewRequest("GET", "http://node.example/api/v1/healthz", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Forwarded-Proto", "https")
		if qualified {
			request.Header.Set("X-Homenode-Client-Address", "100.64.0.2")
			request.Header.Set("X-Homenode-Client-TLS", "1.3")
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !qualified && response.StatusCode != http.StatusForbidden {
			t.Fatal("unqualified Unix request accepted")
		}
		if qualified && (response.StatusCode != http.StatusOK || !strings.Contains(string(body), "alive") || response.Header.Get("Strict-Transport-Security") == "") {
			t.Fatal("verified gateway failed production controller guard", response.StatusCode, string(body))
		}
	}
}

func TestProductionControllerAuthorizationOverRealGatewayTLS(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("unprivileged same-identity fixture")
	}
	controller := testServer(t)
	controller.config.Development = false
	controller.config.Origin, controller.host = "https://example.com", "example.com"
	token := seedSession(t, controller, `["files"]`)
	handler, err := gatewaytransport.ControllerHandler(uint32(os.Geteuid()), controller)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socket, 0660); err != nil {
		t.Fatal(err)
	}
	private := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- private.Serve(listener) }()
	defer func() { private.Close(); <-done }()
	proxy, closeProxy, err := gatewaytransport.NewProxy(gatewaytransport.Config{Origin: controller.config.Origin, Socket: socket, ControllerUID: uint32(os.Geteuid()), AccessGID: uint32(os.Getegid())})
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	public := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The TCP connection is real loopback TLS; overlay routing is not part
		// of this fixture. Preserve the actual TLS state supplied by net/http.
		r.RemoteAddr = "100.64.0.2:12345"
		proxy.ServeHTTP(w, r)
	}))
	public.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	public.StartTLS()
	defer public.Close()
	roots := x509.NewCertPool()
	roots.AddCert(public.Certificate())
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", public.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, entry := range []struct {
		path          string
		authenticated bool
		status        int
	}{
		{"/api/v1/healthz", false, 200}, {"/api/v1/files", false, 401},
		{"/api/v1/files", true, 200}, {"/api/v1/devices", true, 403},
	} {
		request, err := http.NewRequest("GET", controller.config.Origin+entry.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Homenode-Client-Tls", "unsafe")
		request.Header.Set("Forwarded", "for=100.64.0.99;proto=http")
		if entry.authenticated {
			request.AddCookie(&http.Cookie{Name: controller.cookie, Value: token})
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(entry.path, err)
		}
		_, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if readErr != nil || response.StatusCode != entry.status || response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || len(response.TLS.VerifiedChains) == 0 || response.Header.Get("Strict-Transport-Security") == "" || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("TLS/controller authorization mismatch", entry.path, response.StatusCode, entry.status, readErr)
		}
	}
}
