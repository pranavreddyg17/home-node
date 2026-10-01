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
