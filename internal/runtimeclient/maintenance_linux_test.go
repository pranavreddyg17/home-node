//go:build linux

package runtimeclient

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestMaintenanceSocketKernelPeerAndJournalRestart(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("backup peer must be an unprivileged UID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	directory := t.TempDir()
	journal := filepath.Join(directory, "journal")
	store, err := state.Open(journal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	manager := func(store *state.Store) *supervisor.Manager {
		m := &supervisor.Manager{Store: store, Policy: supervisor.Policy{Generation: 1, MemoryMiB: 4096, VCPUs: 4, MaxInstances: 2, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: uid + 1, TransferUID: uid + 2}, Images: directory, Volumes: directory, Channels: directory}
		if err := m.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
		return m
	}
	socket := filepath.Join(directory, "maintenance.sock")
	serve := func(handler http.Handler) (*MaintenanceClient, func()) {
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 5 * time.Second}
		done := make(chan struct{})
		go func() { defer close(done); _ = server.Serve(listener) }()
		client := NewMaintenance(socket)
		return client, func() { client.client.CloseIdleConnections(); server.Close(); <-done }
	}
	m := manager(store)
	client, stop := serve(m.MaintenanceHandler(uid))
	job := state.Random()
	token, err := client.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	replay, err := client.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil || replay != token {
		stop()
		t.Fatal("live acquisition replay", replay, err)
	}
	stop()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	store, err = state.Open(journal)
	if err != nil {
		t.Fatal(err)
	}
	m = manager(store)
	client, stop = serve(m.MaintenanceHandler(uid))
	replay, err = client.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil || replay != token {
		stop()
		t.Fatal("restart lost ownership", replay, err)
	}
	for i := 0; i < 2; i++ {
		if err = client.EndRuntimeMaintenance(ctx, token); err != nil {
			stop()
			t.Fatal("release acknowledgement replay", err)
		}
	}
	if _, err = client.BeginRuntimeMaintenanceForJob(ctx, job); err == nil {
		stop()
		t.Fatal("released job reacquired")
	}
	stop()
	// A real kernel peer with a different configured backup role is refused,
	// independently of correctly formed requests and valid stable job IDs.
	client, stop = serve(m.MaintenanceHandler(uid + 3))
	defer stop()
	if _, err = client.BeginRuntimeMaintenanceForJob(ctx, state.Random()); err == nil {
		t.Fatal("foreign kernel peer accepted")
	}
	var barriers int
	if err = store.DB.QueryRowContext(ctx, "SELECT count(*) FROM settings WHERE key='runtime.maintenance'").Scan(&barriers); err != nil || barriers != 0 {
		t.Fatal("denied peer mutated admission", barriers, err)
	}
}
