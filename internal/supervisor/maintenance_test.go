package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRuntimeMaintenancePreservesBarrierAcrossManagerReconstruction(t *testing.T) {
	m, b := newManager(t)
	ctx := context.Background()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BeginRuntimeMaintenance(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("running runtime accepted", err)
	}
	stop := Request{Version: 1, OperationID: state.Random(), Action: "stop", InstanceID: start.InstanceID, Revision: 2, PolicyGeneration: 1}
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	token, err := m.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.BeginRuntimeMaintenance(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("overlapping barrier accepted", err)
	}
	restarted := &Manager{Store: m.Store, Policy: m.Policy, Manifest: m.Manifest, Images: m.Images, Volumes: m.Volumes, Channels: m.Channels, Backend: b}
	if err = restarted.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Request{start, stop, startRequest()} {
		if _, err = restarted.Apply(ctx, r); !errors.Is(err, ErrPolicy) {
			t.Fatal("barrier bypassed", r, err)
		}
	}
	if b.starts != 1 || b.stops != 1 {
		t.Fatal("blocked runtime effects", b.starts, b.stops)
	}
	if _, err = restarted.Inspect(ctx, start.InstanceID); err != nil {
		t.Fatal("inspection blocked", err)
	}
	if err = restarted.EndRuntimeMaintenance(ctx, state.Random()); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign release accepted", err)
	}
	if err = restarted.EndRuntimeMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	next := start
	next.OperationID = state.Random()
	next.Revision = 3
	if _, err = restarted.Apply(ctx, next); err != nil {
		t.Fatal("admission did not reopen", err)
	}
	if err = restarted.EndRuntimeMaintenance(ctx, token); !errors.Is(err, ErrPolicy) {
		t.Fatal("released token accepted", err)
	}
}

func TestRuntimeMaintenanceBlocksShutdownAndVideoPurge(t *testing.T) {
	for _, action := range []string{"shutdown", "purge"} {
		t.Run(action, func(t *testing.T) {
			m, b := newManager(t)
			ctx := context.Background()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "stop", InstanceID: start.InstanceID, Revision: 2, PolicyGeneration: 1}); err != nil {
				t.Fatal(err)
			}
			backend := &shutdownFixtureBackend{fakeBackend: b}
			m.Backend = backend
			volume := filepath.Join(m.Volumes, start.InstanceID+".raw")
			if action == "purge" {
				if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET workload='video' WHERE id=?", start.InstanceID); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(volume, []byte("retained"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := m.BeginRuntimeMaintenance(ctx); err != nil {
				t.Fatal(err)
			}
			r := Request{Version: 1, OperationID: state.Random(), Action: action, InstanceID: start.InstanceID, Revision: 3, PolicyGeneration: 1}
			if _, err := m.Apply(ctx, r); !errors.Is(err, ErrPolicy) {
				t.Fatal("mutation accepted", err)
			}
			var count int
			if err := m.Store.DB.QueryRow("SELECT count(*) FROM runtime_operations WHERE id=?", r.OperationID).Scan(&count); err != nil || count != 0 {
				t.Fatal("blocked mutation journaled", count, err)
			}
			if backend.shutdowns != 0 {
				t.Fatal("shutdown executed")
			}
			if action == "purge" {
				if data, err := os.ReadFile(volume); err != nil || string(data) != "retained" {
					t.Fatal("purge touched data", string(data), err)
				}
			}
		})
	}
}

func TestRuntimeMaintenanceRefusesUncertainRecordedWork(t *testing.T) {
	for _, scenario := range []string{"pending-operation", "unknown-operation", "unknown-instance", "wrong-desired"} {
		t.Run(scenario, func(t *testing.T) {
			m, _ := newManager(t)
			ctx := context.Background()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), Action: "stop", InstanceID: start.InstanceID, Revision: 2, PolicyGeneration: 1}); err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "pending-operation":
				_, err = m.Store.DB.Exec("UPDATE runtime_operations SET state='pending' WHERE id=?", start.OperationID)
			case "unknown-operation":
				_, err = m.Store.DB.Exec("UPDATE runtime_operations SET state='unknown' WHERE id=?", start.OperationID)
			case "unknown-instance":
				_, err = m.Store.DB.Exec("UPDATE runtime_instances SET state='unknown' WHERE id=?", start.InstanceID)
			case "wrong-desired":
				_, err = m.Store.DB.Exec("UPDATE runtime_instances SET desired='running' WHERE id=?", start.InstanceID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if token, err := m.BeginRuntimeMaintenance(ctx); !errors.Is(err, ErrPolicy) || token != "" {
				t.Fatal("uncertain inventory admitted", token, err)
			}
			var count int
			if err := m.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed acquisition retained barrier", count, err)
			}
		})
	}
}
