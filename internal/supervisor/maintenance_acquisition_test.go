package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRuntimeMaintenanceJobAcquisitionRetryAndReleasedReplay(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	job := state.Random()
	token, err := m.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := m.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil || retry != token {
		t.Fatal("lost acknowledgement created second barrier", retry, err)
	}
	if _, err = m.BeginRuntimeMaintenanceForJob(ctx, state.Random()); !errors.Is(err, ErrPolicy) {
		t.Fatal("different job took authority", err)
	}
	if err = m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	m.Store = reopened
	if err = m.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err = m.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	retry, err = m.BeginRuntimeMaintenanceForJob(ctx, job)
	if err != nil || retry != token {
		t.Fatal("reconciliation lost active job", retry, err)
	}
	if err = m.EndRuntimeMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = m.BeginRuntimeMaintenanceForJob(ctx, job); !errors.Is(err, ErrPolicy) {
		t.Fatal("released job reacquired authority", err)
	}
	next, err := m.BeginRuntimeMaintenanceForJob(ctx, state.Random())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.EndRuntimeMaintenance(ctx, token); err != nil {
		t.Fatal("old release acknowledgement refused", err)
	}
	var current string
	if err = m.Store.DB.QueryRow("SELECT value FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&current); err != nil || current != next {
		t.Fatal("old receipt removed new barrier", current, err)
	}
}

func TestRuntimeMaintenanceJobCheckpointIsAtomic(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	job := state.Random()
	if _, err := m.Store.DB.Exec("CREATE TRIGGER fixture_acquisition BEFORE INSERT ON settings WHEN NEW.key='runtime.maintenance' BEGIN SELECT RAISE(ABORT,'fixture barrier failure'); END"); err != nil {
		t.Fatal(err)
	}
	if token, err := m.BeginRuntimeMaintenanceForJob(ctx, job); err == nil || token != "" {
		t.Fatal("partial checkpoint succeeded", token, err)
	}
	var count int
	if err := m.Store.DB.QueryRow("SELECT (SELECT count(*) FROM settings WHERE key IN('runtime.maintenance','runtime.maintenance-job'))+(SELECT count(*) FROM runtime_operations WHERE id=?)", job).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial acquisition retained authority", count, err)
	}
}
