package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceRecoveryPreservesUnacknowledgedAcquisitionUntilBridgeReturns(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "management"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	device := state.Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	apps := &coordinatorApps{}
	unavailable := errors.New("fixture acquisition unavailable")
	root := &coordinatorRoot{acquireErr: unavailable}
	callbacks := 0
	work := func(context.Context, string, string) error { callbacks++; return nil }
	id, err := RunMaintenance(ctx, store, device, apps, root, work, work)
	if !errors.Is(err, unavailable) || id == "" || callbacks != 0 {
		t.Fatal(id, callbacks, err)
	}
	var token string
	if err = store.DB.QueryRow("SELECT value FROM settings WHERE key='host.maintenance'").Scan(&token); err != nil {
		t.Fatal(err)
	}
	job, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil || job.Phase != "freezing" || job.RootToken != "" {
		t.Fatal("uncertain acquisition forgotten", job, err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, id, "freezing", "restoring"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("unacknowledged acquisition skipped", err)
	}
	if err = RecoverMaintenance(ctx, store, token, state.Random(), apps, root); !errors.Is(err, state.ErrMaintenanceOwner) {
		t.Fatal("wrong job recovered", err)
	}
	if apps.restored || root.released {
		t.Fatal("cleanup crossed uncertain root ownership")
	}
	root.acquireErr = nil
	if err = RecoverMaintenance(ctx, store, token, id, apps, root); err != nil {
		t.Fatal(err)
	}
	if !apps.restored || !root.released || callbacks != 0 {
		t.Fatal("recovery replayed backup work or skipped cleanup", apps, root, callbacks)
	}
	if err = store.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal("recovery retained barrier", err)
	}
}

func TestMaintenanceRecoveryRetriesCleanupWithoutRepeatingPublication(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "management"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	device := state.Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("fixture restoration failed")
	apps := &coordinatorApps{restoreErr: failure}
	root := &coordinatorRoot{}
	calls := 0
	work := func(context.Context, string, string) error { calls++; return nil }
	id, err := RunMaintenance(ctx, store, device, apps, root, work, work)
	if !errors.Is(err, failure) || calls != 2 {
		t.Fatal(id, calls, err)
	}
	var token string
	if err = store.DB.QueryRow("SELECT value FROM settings WHERE key='host.maintenance'").Scan(&token); err != nil {
		t.Fatal(err)
	}
	retained, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil || retained.Phase != "requires-action" || retained.RootToken != "" {
		t.Fatal(retained, err)
	}
	apps.restoreErr = nil
	if err = RecoverMaintenance(ctx, store, token, id, apps, root); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || root.acquires != 1 {
		t.Fatal("recovery repeated completed work", calls, root.acquires)
	}
	if err = store.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceRunnerExcludesRecoveryWhileStaging(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "management")
	store, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	second, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	device := state.Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	apps := &coordinatorApps{}
	root := &coordinatorRoot{}
	stage := func(ctx context.Context, token, rootToken string) error {
		job, err := second.InspectMaintenanceJob(ctx, token)
		if err != nil {
			return err
		}
		if err = RecoverMaintenance(ctx, second, token, job.ID, apps, root); !errors.Is(err, ErrMaintenanceRunner) {
			t.Fatalf("recovery entered live staging: %v", err)
		}
		work := func(context.Context, string, string) error { return nil }
		if _, err = RunMaintenance(ctx, second, device, apps, root, work, work); !errors.Is(err, ErrMaintenanceRunner) {
			t.Fatalf("second runner entered live staging: %v", err)
		}
		if apps.restored || root.released {
			t.Fatal("competing runner performed cleanup")
		}
		current, err := second.InspectMaintenanceJob(ctx, token)
		if err != nil || current.Phase != "staging" {
			t.Fatal("competing runner mutated journal", current, err)
		}
		return nil
	}
	if _, err = RunMaintenance(context.Background(), store, device, apps, root, stage, func(context.Context, string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !apps.restored || !root.released {
		t.Fatal("owner did not finish cleanup")
	}
}
