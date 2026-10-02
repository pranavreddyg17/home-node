package state

import (
	"context"
	"testing"
)

func TestBackupDispatchIntentSurvivesRestartAndBlocksCleanup(t *testing.T) {
	store, device, directory := maintenanceJobFixture(t)
	defer func() { store.Close() }()
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
	if err = store.ClaimBackupDispatch(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupDispatch(ctx, token, job.ID); err == nil {
		t.Fatal("dispatch replay admitted")
	}
	if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err == nil {
		t.Fatal("completion before publication admitted")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RequireBackupWorkerStopped(ctx, token, job.ID); err == nil {
		t.Fatal("restart forgot uncertain dispatch")
	}
	if observation, observeErr := store.InspectBackupObservation(ctx); observeErr != nil || observation.WorkerCompletion != "uncertain" || observation.Current != nil {
		t.Fatal("restart uncertainty not observable", observation, observeErr)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "restoring"); err == nil {
		t.Fatal("uncertain dispatch allowed restoration")
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupPublication(ctx, token, job.ID, device); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordBackupPublished(ctx, token, job.ID, device, Hash("fixture snapshot")); err != nil {
		t.Fatal(err)
	}
	if observation, observeErr := store.InspectBackupObservation(ctx); observeErr != nil || observation.WorkerCompletion != "uncertain" || observation.Current == nil || observation.Current.Status != "published" {
		t.Fatal("publication obscured completion uncertainty", observation, observeErr)
	}
	if err = store.RequireBackupWorkerStopped(ctx, token, job.ID); err == nil {
		t.Fatal("publication substituted for completion")
	}
	if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if observation, observeErr := store.InspectBackupObservation(ctx); observeErr != nil || observation.WorkerCompletion != "complete" {
		t.Fatal("restart lost completed worker evidence", observation, observeErr)
	}
	if err = store.RequireBackupWorkerStopped(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "publishing", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = store.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = store.CompleteMaintenanceJob(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.BeginMaintenanceJob(ctx, device); err != nil {
		t.Fatal("completion retained obsolete dispatch marker", err)
	}
}

func TestOrphanDispatchRefusesGeneralAdmissionAndLegacyRelease(t *testing.T) {
	for _, value := range []string{"uncertain:" + Random(), "complete:" + Random(), "malformed"} {
		t.Run(value, func(t *testing.T) {
			store, device, _ := maintenanceJobFixture(t)
			defer store.Close()
			ctx := context.Background()
			token, err := store.BeginMaintenance(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.DB.Exec("INSERT INTO settings(key,value) VALUES(?,?)", backupDispatchKey, value); err != nil {
				t.Fatal(err)
			}
			if err = store.EndMaintenance(ctx, token); err == nil {
				t.Fatal("legacy release ignored worker checkpoint")
			}
			var retained string
			if err = store.DB.QueryRow("SELECT value FROM settings WHERE key=?", maintenanceKey).Scan(&retained); err != nil || retained != token {
				t.Fatal("failed release changed barrier", retained, err)
			}
			// Model incomplete/corrupt ownership state: checkpoint alone must still
			// quarantine all ordinary transactional workload admission.
			if _, err = store.DB.Exec("DELETE FROM settings WHERE key=?", maintenanceKey); err != nil {
				t.Fatal(err)
			}
			if err = store.Transaction(ctx, RequireAdmission); err == nil {
				t.Fatal("orphan checkpoint reopened workload admission")
			}
			if _, err = store.BeginMaintenance(ctx); err == nil {
				t.Fatal("orphan checkpoint admitted legacy maintenance")
			}
			if _, _, err = store.BeginMaintenanceJob(ctx, device); err == nil {
				t.Fatal("orphan checkpoint admitted another coordinator")
			}
			if observation, observeErr := store.InspectBackupObservation(ctx); observeErr == nil || observation.Current != nil || observation.WorkerCompletion != "" {
				t.Fatal("orphan checkpoint reported healthy status", observation, observeErr)
			}
		})
	}
}

func TestPreliminaryLaunchIntentSurvivesRestartAndBlocksCleanup(t *testing.T) {
	store, device, directory := maintenanceJobFixture(t)
	defer func() { store.Close() }()
	ctx := context.Background()
	token, job, err := store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupLaunch(ctx, token, job.ID); err == nil {
		t.Fatal("draining launch admitted")
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupLaunch(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimBackupLaunch(ctx, token, job.ID); err == nil {
		t.Fatal("launch replay admitted")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RequireBackupWorkerStopped(ctx, token, job.ID); err == nil {
		t.Fatal("restart lost launch uncertainty")
	}
	observation, err := store.InspectBackupObservation(ctx)
	if err != nil || observation.WorkerCompletion != "uncertain" {
		t.Fatal("pre-acquisition launch status unavailable", observation, err)
	}
	if err = store.AttachMaintenanceRoot(ctx, token, job.ID, Random()); err != nil {
		t.Fatal("worker acquisition checkpoint refused", err)
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "restoring"); err == nil {
		t.Fatal("active preliminary worker allowed restoration")
	}
	if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err == nil {
		t.Fatal("unpublished worker completion admitted")
	}
}
