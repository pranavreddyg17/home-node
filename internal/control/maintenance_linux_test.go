//go:build linux

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestControllerMaintenanceKernelPeerAndOwnedJob(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("backup peer must be unprivileged")
	}
	server := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	device := state.Random()
	if _, err := server.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store.DB.Exec("UPDATE identity SET owner_id=?,claimed=1,epoch=1 WHERE singleton=1", state.Random()); err != nil {
		t.Fatal(err)
	}
	token, job, err := server.Store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "maintenance.sock"))
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: server.MaintenanceHandler(uid+1, uid), ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	defer func() { httpServer.Close(); <-done }()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	call := func(path string, request maintenanceRequest, expected int) {
		t.Helper()
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "http://local"+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 513))
		response.Body.Close()
		if readErr != nil || response.StatusCode != expected {
			t.Fatal("unexpected maintenance response", response.StatusCode, string(body), readErr)
		}
	}
	request := maintenanceRequest{Version: 1, Token: token, JobID: job.ID, DeviceID: device}
	bridge := runtimeclient.NewMaintenanceApps(listener.Addr().String(), uid, server.Store.InspectMaintenanceJob)
	defer bridge.Close()
	foreignPeer := runtimeclient.NewMaintenanceApps(listener.Addr().String(), uid+2, server.Store.InspectMaintenanceJob)
	defer foreignPeer.Close()
	if err = foreignPeer.DrainMaintenance(ctx, token, device); err == nil {
		t.Fatal("client trusted foreign controller UID")
	}
	foreign := request
	foreign.JobID = state.Random()
	call("/v1/maintenance/drain", foreign, 409)
	foreign = request
	foreign.DeviceID = state.Random()
	call("/v1/maintenance/drain", foreign, 409)
	foreign = request
	foreign.Token = state.Random()
	call("/v1/maintenance/drain", foreign, 409)
	call("/v1/runtime/apply", request, 404)
	if err = bridge.DrainMaintenance(ctx, token, device); err != nil {
		t.Fatal("owned client drain", err)
	}
	call("/v1/maintenance/restore", request, 409)
	call("/v1/maintenance/verify-stage", request, 409)
	if err = bridge.ClaimPublication(ctx, token, device); err != nil {
		t.Fatal("publication claim", err)
	}
	if err = bridge.ClaimPublication(ctx, token, device); err == nil {
		t.Fatal("repeated publication claim accepted")
	}
	if err = server.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if err = server.Store.AttachMaintenanceRoot(ctx, token, job.ID, state.Random()); err != nil {
		t.Fatal(err)
	}
	snapshotDirectory := t.TempDir()
	if err = os.Chmod(snapshotDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	staging, err := os.OpenRoot(snapshotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Close()
	owned, err := server.Store.InspectMaintenanceJob(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	policy := backup.RestorePolicy{MinimumCatalogVersion: 1}
	manifest, err := backup.StagePrivateRecoverySet(ctx, bridge, runtimeclient.NewDiskClient("/tmp/unused-empty-inventory.sock"), device, token, owned.RootToken, staging, "0.1.0", 1, policy)
	if err != nil || len(manifest.Files) != 1 {
		t.Fatal("private recovery staging", manifest, err)
	}
	if err = backup.ValidateRecoverySet(ctx, staging, manifest, policy); err != nil {
		t.Fatal("private staged recovery set", err)
	}
	snapshot, err := staging.Open("snapshot.db")
	if err != nil {
		t.Fatal(err)
	}
	_, validateErr := state.ValidateRecoverySnapshot(ctx, snapshot)
	snapshot.Close()
	if validateErr != nil {
		t.Fatal("private snapshot authority", validateErr)
	}
	if err = bridge.BeginPublishing(ctx, token, device); err != nil {
		t.Fatal("publish transition", err)
	}
	if err = bridge.BeginPublishing(ctx, token, device); err != nil {
		t.Fatal("lost transition acknowledgement replay", err)
	}
	if err = bridge.ConfirmPublishing(ctx, token, device); err != nil {
		t.Fatal("publish confirmation", err)
	}
	call("/v1/maintenance/verify-stage", request, 409)
	if err = bridge.ClaimPublication(ctx, token, device); err != nil {
		t.Fatal("publication claim", err)
	}
	if err = bridge.ClaimPublication(ctx, token, device); err == nil {
		t.Fatal("repeated publication claim accepted")
	}
	if err = server.Store.AdvanceMaintenanceJob(ctx, token, job.ID, "publishing", "restoring"); err != nil {
		t.Fatal(err)
	}
	call("/v1/maintenance/drain", request, 409)
	call("/v1/maintenance/restore", request, 409)
	if err = server.Store.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = bridge.RestoreMaintenanceApps(ctx, token, device); err != nil {
		t.Fatal("owned client restore", err)
	}
	current, err := server.Store.InspectMaintenanceJob(ctx, token)
	if err != nil || current.ID != job.ID || current.Phase != "restoring" || current.RootToken != "" {
		t.Fatal("handler altered job authority", current, err)
	}
	if err = server.Store.Transaction(ctx, state.RequireAdmission); err == nil {
		t.Fatal("handler released management barrier")
	}
}
