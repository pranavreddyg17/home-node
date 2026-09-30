package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMaintenanceSnapshotRequiresOwnerAndDrainedInventory(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err = s.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,1)", Random()); err != nil {
		t.Fatal(err)
	}
	token, err := s.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	if err = os.Chmod(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceRecoverySnapshot(ctx, Random(), stage); !errors.Is(err, ErrMaintenanceOwner) {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(stage, "snapshot.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("foreign owner created snapshot", err)
	}
	if _, err = s.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('files',?,'running',1,1)", Random()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceRecoverySnapshot(ctx, token, stage); !errors.Is(err, ErrMaintenance) {
		t.Fatal("running app admitted", err)
	}
	if _, err = os.Stat(filepath.Join(stage, "snapshot.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("undrained snapshot created", err)
	}
	if _, err = s.DB.Exec("UPDATE apps SET state='stopped'"); err != nil {
		t.Fatal(err)
	}
	path, err := s.MaintenanceRecoverySnapshot(ctx, token, stage)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	apps, err := ValidateRecoverySnapshot(ctx, file)
	if err != nil || len(apps) != 1 {
		t.Fatal(apps, err)
	}
	if _, err = s.InspectMaintenance(ctx, token); err != nil {
		t.Fatal("snapshot released barrier", err)
	}
}
