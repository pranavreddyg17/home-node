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
	if err = store.RequireBackupWorkerStopped(ctx, token, job.ID); err == nil {
		t.Fatal("publication substituted for completion")
	}
	if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
		t.Fatal(err)
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
