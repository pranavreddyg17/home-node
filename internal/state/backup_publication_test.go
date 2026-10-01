package state

import (
	"context"
	"errors"
	"testing"
)

func TestBackupPublicationClaimSurvivesRestartAndRejectsReplay(t *testing.T) {
	store, device, directory := maintenanceJobFixture(t)
	ctx := context.Background()
	token, job, err := store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); !errors.Is(err, ErrBackupPublication) {
		t.Fatal("non-publishing claim accepted", err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if err = store.AttachMaintenanceRoot(ctx, token, job.ID, Random()); err != nil {
		t.Fatal(err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupPublication(ctx, token, Random(), device); !errors.Is(err, ErrBackupPublication) {
		t.Fatal("foreign job accepted", err)
	}
	if _, err = store.DB.Exec("INSERT INTO settings VALUES('host.activity.claim-fixture','active')"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); !errors.Is(err, ErrBackupPublication) {
		t.Fatal("active inventory publication accepted", err)
	}
	if _, err = store.DB.Exec("DELETE FROM settings WHERE key='host.activity.claim-fixture'"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); !errors.Is(err, ErrBackupPublication) {
		t.Fatal("restart permitted repeated publication", err)
	}
	var persisted string
	if err = store.DB.QueryRow("SELECT value FROM settings WHERE key=?", backupPublicationKey).Scan(&persisted); err != nil || persisted != job.ID {
		t.Fatal("publication intent lost", persisted, err)
	}
}

func TestBackupPublicationOutcomeAcknowledgementAndRestart(t *testing.T) {
	store, device, directory := maintenanceJobFixture(t)
	ctx := context.Background()
	token, job, err := store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if err = store.AttachMaintenanceRoot(ctx, token, job.ID, Random()); err != nil {
		t.Fatal(err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
		t.Fatal(err)
	}
	snapshot := Hash("validated repository snapshot")
	if err = store.RecordBackupPublished(ctx, token, job.ID, device, snapshot); err == nil {
		t.Fatal("unclaimed acknowledgement accepted")
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); err != nil {
		t.Fatal(err)
	}
	current, success, err := store.InspectBackupOutcomes(ctx)
	if err != nil || current == nil || current.Status != "unknown" || success != nil {
		t.Fatal("claim outcome", current, success, err)
	}
	for _, invalid := range []struct{ id, device, snapshot string }{{Random(), device, snapshot}, {job.ID, Random(), snapshot}, {job.ID, device, "short"}} {
		if err = store.RecordBackupPublished(ctx, token, invalid.id, invalid.device, invalid.snapshot); err == nil {
			t.Fatal("invalid acknowledgement accepted")
		}
	}
	if err = store.RecordBackupPublished(ctx, token, job.ID, device, snapshot); err != nil {
		t.Fatal(err)
	}
	current, success, err = store.InspectBackupOutcomes(ctx)
	if err != nil || current == nil || success == nil || *current != *success || current.SnapshotID != snapshot || current.Status != "published" {
		t.Fatal("published outcome", current, success, err)
	}
	original := *current
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.RecordBackupPublished(ctx, token, job.ID, device, snapshot); err != nil {
		t.Fatal("lost acknowledgement replay", err)
	}
	if err = store.RecordBackupPublished(ctx, token, job.ID, device, Hash("different")); err == nil {
		t.Fatal("outcome overwritten")
	}
	current, success, err = store.InspectBackupOutcomes(ctx)
	if err != nil || *current != original || *success != original {
		t.Fatal("outcome lost after restart", err)
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); err == nil {
		t.Fatal("acknowledgement enabled publication replay")
	}
}

func TestBackupOutcomeMalformedRecordRefused(t *testing.T) {
	store, _, _ := maintenanceJobFixture(t)
	for _, raw := range []string{`null`, `{}`, `{"version":1,"version":1}`, `{"status":"published"}`} {
		if _, err := store.DB.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", backupOutcomeKey, raw); err != nil {
			t.Fatal(err)
		}
		if current, success, err := store.InspectBackupOutcomes(context.Background()); err == nil || current != nil || success != nil {
			t.Fatal("malformed outcome accepted", raw)
		}
	}
}

func TestUncertainPublicationCannotBeReplacedByNewClaim(t *testing.T) {
	for _, acknowledged := range []bool{false, true} {
		store, device, _ := maintenanceJobFixture(t)
		ctx := context.Background()
		claim := func() (string, MaintenanceJob) {
			t.Helper()
			token, job, err := store.BeginMaintenanceJob(ctx, device)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
				t.Fatal(err)
			}
			if err = store.AttachMaintenanceRoot(ctx, token, job.ID, Random()); err != nil {
				t.Fatal(err)
			}
			if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
				t.Fatal(err)
			}
			return token, job
		}
		token, first := claim()
		if err := store.ClaimBackupPublication(ctx, token, first.ID, device); err != nil {
			t.Fatal(err)
		}
		if acknowledged {
			if err := store.RecordBackupPublished(ctx, token, first.ID, device, Hash("first snapshot")); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.AdvanceMaintenanceJob(ctx, token, first.ID, "publishing", "restoring"); err != nil {
			t.Fatal(err)
		}
		if err := store.ReleaseMaintenanceRoot(ctx, token, first.ID, func(context.Context, string) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := store.CompleteMaintenanceJob(ctx, token, first.ID); err != nil {
			t.Fatal(err)
		}
		token, second := claim()
		err := store.ClaimBackupPublication(ctx, token, second.ID, device)
		if acknowledged && err != nil || !acknowledged && !errors.Is(err, ErrBackupPublication) {
			t.Fatal("replacement policy", acknowledged, err)
		}
		current, success, err := store.InspectBackupOutcomes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if acknowledged {
			if current.JobID != second.ID || current.Status != "unknown" || success == nil || success.JobID != first.ID {
				t.Fatal("last success not retained")
			}
		} else if current.JobID != first.ID || current.Status != "unknown" || success != nil {
			t.Fatal("uncertain outcome replaced")
		}
	}
}
