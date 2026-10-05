//go:build linux

package control

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestSnapshotManagementRequiresKernelPeerAndPendingRequest(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("backup peer must be unprivileged")
	}
	s := testServer(t)
	token := seedBackupSession(t, s)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	actor, err := s.Identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	request, release, err := s.beginSnapshotRequest(context.Background(), actor, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "management.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: s.MaintenanceHandler(uid+1, uid), ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	call := func(message backup.SnapshotPageRequest, want int) {
		t.Helper()
		raw, err := backup.EncodeSnapshotPageRequest(message)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Post("http://local/v1/maintenance/verify-snapshot-page", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 513))
		if err != nil || response.StatusCode != want {
			t.Fatal(response.StatusCode, string(body), err)
		}
		if want == 200 && (string(body) != "{\"version\":1}\n" || response.Header.Get("Cache-Control") != "no-store") {
			t.Fatal("invalid acknowledgement", string(body))
		}
	}
	call(request, 200)
	call(request, 200)
	foreign := request
	foreign.Cursor = state.Hash("foreign")
	call(foreign, 403)
	release()
	call(request, 403)
	if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
		t.Fatal("selector acquired maintenance", err)
	}
}
