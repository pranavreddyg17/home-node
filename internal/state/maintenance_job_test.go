package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func maintenanceJobFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "state")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	device := Random()
	if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	return store, device, directory
}

func TestMaintenanceJobPersistsOwnerPhaseAndRootAuthority(t *testing.T) {
	s, device, directory := maintenanceJobFixture(t)
	ctx := context.Background()
	token, job, err := s.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "publishing"); !errors.Is(err, ErrMaintenance) {
		t.Fatal("phase skipped", err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	rootToken := Random()
	if err = s.AttachMaintenanceRoot(ctx, token, job.ID, rootToken); err != nil {
		t.Fatal(err)
	}
	if err = s.AttachMaintenanceRoot(ctx, token, job.ID, Random()); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("root token replaced", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.InspectMaintenanceJob(ctx, token)
	if err != nil || restored.ID != job.ID || restored.Device != device || restored.Phase != "staging" || restored.RootToken != rootToken {
		t.Fatal(restored, err)
	}
	if err = s.EndMaintenance(ctx, token); !errors.Is(err, ErrMaintenance) {
		t.Fatal("journal barrier released", err)
	}
	if err = s.Transaction(ctx, RequireAdmission); !errors.Is(err, ErrMaintenance) {
		t.Fatal("restart reopened admission", err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("stale phase accepted", err)
	}
	if _, err = s.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", device); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "restoring", "requires-action"); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("revoked owner advanced journal", err)
	}
}

func TestMaintenanceJobAdmissionRollbackAndUnknownFieldRefusal(t *testing.T) {
	s, device, _ := maintenanceJobFixture(t)
	defer s.Close()
	ctx := context.Background()
	if _, err := s.DB.Exec("CREATE TRIGGER fixture_job BEFORE INSERT ON settings WHEN NEW.key='host.maintenance-job.phase' BEGIN SELECT RAISE(ABORT,'fixture write failure'); END"); err != nil {
		t.Fatal(err)
	}
	if token, _, err := s.BeginMaintenanceJob(ctx, device); err == nil || token != "" {
		t.Fatal("partial begin succeeded", token, err)
	}
	var count int
	if err := s.DB.QueryRow("SELECT count(*) FROM settings WHERE key='host.maintenance' OR key GLOB 'host.maintenance-job.*'").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial authority persisted", count, err)
	}
	if _, err := s.DB.Exec("DROP TRIGGER fixture_job"); err != nil {
		t.Fatal(err)
	}
	token, job, err := s.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO settings VALUES('host.maintenance-job.unrecognized','retained')"); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); !errors.Is(err, ErrMaintenance) {
		t.Fatal("unknown record field ignored", err)
	}
	if _, err = s.DB.Exec("DELETE FROM settings WHERE key='host.maintenance'"); err != nil {
		t.Fatal(err)
	}
	if err = s.Transaction(ctx, RequireAdmission); !errors.Is(err, ErrMaintenance) {
		t.Fatal("orphan journal reopened admission", err)
	}
}

func TestMaintenanceRootCheckpointRollsBackWithPhaseWriteFailure(t *testing.T) {
	s, device, _ := maintenanceJobFixture(t)
	defer s.Close()
	ctx := context.Background()
	token, job, err := s.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("CREATE TRIGGER fixture_root BEFORE UPDATE ON settings WHEN NEW.key='host.maintenance-job.phase' AND NEW.value='staging' BEGIN SELECT RAISE(ABORT,'fixture phase write failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err = s.AttachMaintenanceRoot(ctx, token, job.ID, Random()); err == nil {
		t.Fatal("partial checkpoint succeeded")
	}
	retained, err := s.InspectMaintenanceJob(ctx, token)
	if err != nil || retained.Phase != "freezing" || retained.RootToken != "" {
		t.Fatal("partial root authority persisted", retained, err)
	}
}

func TestMaintenanceJobCompletionWaitsForRootRelease(t *testing.T) {
	s, device, _ := maintenanceJobFixture(t)
	defer s.Close()
	ctx := context.Background()
	token, job, err := s.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	rootToken := Random()
	if err = s.AttachMaintenanceRoot(ctx, token, job.ID, rootToken); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteMaintenanceJob(ctx, token, job.ID); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("root authority forgotten", err)
	}
	fixtureError := errors.New("fixture root release failed")
	if err = s.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return fixtureError }); !errors.Is(err, fixtureError) {
		t.Fatal(err)
	}
	retained, err := s.InspectMaintenanceJob(ctx, token)
	if err != nil || retained.RootToken != rootToken {
		t.Fatal("failed release lost authority", retained, err)
	}
	if err = s.ReleaseMaintenanceRoot(ctx, token, job.ID, func(_ context.Context, got string) error {
		if got != rootToken {
			t.Fatal(got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO settings VALUES('host.activity.fixture','trash-expiry')"); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteMaintenanceJob(ctx, token, job.ID); !errors.Is(err, ErrMaintenance) {
		t.Fatal("unfinished activity ignored", err)
	}
	if _, err = s.DB.Exec("DELETE FROM settings WHERE key='host.activity.fixture'"); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteMaintenanceJob(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Transaction(ctx, RequireAdmission); err != nil {
		t.Fatal("completed job kept admission closed", err)
	}
	if err = s.CompleteMaintenanceJob(ctx, token, job.ID); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal("old owner reused", err)
	}
}

func TestLegacyAmbiguousMaintenanceRepairRecordFailsClosed(t *testing.T) {
	s, device, _ := maintenanceJobFixture(t)
	defer s.Close()
	ctx := context.Background()
	token, _, err := s.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE settings SET value='1' WHERE key='host.maintenance-job.version'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE settings SET value='requires-action' WHERE key='host.maintenance-job.phase'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InspectMaintenanceJob(ctx, token); !errors.Is(err, ErrMaintenance) {
		t.Fatal("legacy lost acquisition intent accepted", err)
	}
	if err = s.Transaction(ctx, RequireAdmission); !errors.Is(err, ErrMaintenance) {
		t.Fatal("ambiguous record reopened admission", err)
	}
}
