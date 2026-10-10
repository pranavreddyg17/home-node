//go:build linux

package control

import (
	"context"
	"io"
	"net"
	"net/http"
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
