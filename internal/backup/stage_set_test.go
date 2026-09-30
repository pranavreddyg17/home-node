package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type fixtureMaintenanceDisks struct {
	token     string
	source    *os.File
	instances map[string]supervisor.Instance
	failID    string
	after     func()
	calls     int
}

func (d *fixtureMaintenanceDisks) WithMaintenanceDisk(ctx context.Context, token, id string, copyDisk func(context.Context, *os.File, supervisor.Instance) error) error {
	if token != d.token {
		return ErrManifest
	}
	d.calls++
	if id == d.failID {
		return errors.New("fixture disk unavailable")
	}
	instance, ok := d.instances[id]
	if !ok {
		return ErrManifest
	}
	err := copyDisk(ctx, d.source, instance)
	if d.after != nil {
		d.after()
	}
	return err
}

func recoveryStageFixture(t *testing.T) (*state.Store, *fixtureMaintenanceDisks, *os.Root, string, RestorePolicy) {
	t.Helper()
	root, source, entry, _ := diskStageFixture(t)
	store, err := state.Open(filepath.Join(t.TempDir(), "management"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err = store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	disks := &fixtureMaintenanceDisks{token: state.Random(), source: source, instances: map[string]supervisor.Instance{}}
	for _, workload := range []string{"ai", "files"} {
		id := state.Random()
		if _, err = store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES(?,?,'stopped',1,1)", workload, id); err != nil {
			t.Fatal(err)
		}
		disks.instances[id] = supervisor.Instance{ID: id, Workload: workload, State: "stopped", Desired: "stopped", ImageSHA256: entry.ImageSHA256, DataBytes: entry.Bytes}
	}
	token, err := store.BeginMaintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return store, disks, root, token, RestorePolicy{MinimumCatalogVersion: 1, ApprovedImages: map[string]string{"ai": entry.ImageSHA256, "files": entry.ImageSHA256}}
}

func TestStageRecoverySetMatchesSnapshotInventoryAndDeclaredDiskBytes(t *testing.T) {
	store, disks, root, token, policy := recoveryStageFixture(t)
	manifest, err := StageRecoverySet(context.Background(), store, disks, token, disks.token, root, "0.1.0", 1, policy)
	if err != nil || len(manifest.Files) != 3 || disks.calls != 2 {
		t.Fatal(manifest, disks.calls, err)
	}
	if err = ValidateRecoverySet(context.Background(), root, manifest, policy); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectMaintenance(context.Background(), token); err != nil {
		t.Fatal("builder released source admission", err)
	}
}

func TestStageRecoverySetFailureRemovesItsOwnArtifacts(t *testing.T) {
	for _, scenario := range []string{"missing-disk", "wrong-identity", "wrong-image", "owner-lost", "invalid-snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			store, disks, root, token, policy := recoveryStageFixture(t)
			for id, instance := range disks.instances {
				if instance.Workload != "files" {
					continue
				}
				switch scenario {
				case "missing-disk":
					disks.failID = id
				case "wrong-identity":
					instance.ID = state.Random()
					disks.instances[id] = instance
				case "wrong-image":
					instance.ImageSHA256 = state.Hash("unapproved")
					disks.instances[id] = instance
				}
			}
			if scenario == "invalid-snapshot" {
				if _, err := store.DB.Exec("DELETE FROM identity"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "owner-lost" {
				once := false
				disks.after = func() {
					if !once {
						once = true
						if err := store.EndMaintenance(context.Background(), token); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			manifest, err := StageRecoverySet(context.Background(), store, disks, token, disks.token, root, "0.1.0", 1, policy)
			if err == nil || len(manifest.Files) != 0 {
				t.Fatal("incomplete set reported success", manifest, err)
			}
			directory, err := root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			entries, err := directory.ReadDir(-1)
			directory.Close()
			if err != nil || len(entries) != 0 {
				t.Fatal("failed builder retained artifacts", entries, err)
			}
		})
	}
}
