//go:build linux

package backup

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRealResticMaintenanceBackupRoundTrip(t *testing.T) {
	if os.Getenv("HOMENODE_RESTIC_INTEGRATION") != "1" {
		t.Skip("requires disposable Linux restic integration environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store, disks, staging, token, policy := recoveryStageFixture(t)
	if err := store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	device := state.Random()
	if _, err := store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	password := []byte("maintenance roundtrip fixture password")
	if _, err = resticConfig(ctx, parent, password, true); err != nil {
		t.Fatal(err)
	}
	data, err := resticConfig(ctx, parent, password, false)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	target := registeredTarget()
	target.RepositoryID = config.ID
	repository, err := openRepository(ctx, parent, target, password)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	apps := &coordinatorApps{}
	runtime := &coordinatorRoot{token: disks.token}
	result, err := RunBackup(ctx, store, device, apps, runtime, disks, staging, repository, "0.1.0", 1, policy)
	if err != nil || !repositoryPattern.MatchString(result.SnapshotID) || result.JobID == "" {
		t.Fatal(result, err)
	}
	if !apps.restored || !runtime.released {
		t.Fatal("source cleanup skipped")
	}
	if err = store.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal("source admission retained", err)
	}
	if err = repository.Check(ctx); err != nil {
		t.Fatal("encrypted repository integrity", err)
	}
	restored, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	directory, err := restored.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	manifest, err := repository.Restore(ctx, result.SnapshotID, directory, policy)
	if err != nil {
		t.Fatal("restore staged backup", err)
	}
	if len(manifest.Files) != 3 {
		t.Fatal("lost management or app disk inventory", manifest)
	}
	if err = ValidateRecoverySet(ctx, restored, manifest, policy); err != nil {
		t.Fatal("restored recovery set", err)
	}
	snapshot, err := restored.Open("snapshot.db")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	inventory, err := state.ValidateRecoverySnapshot(ctx, snapshot)
	if err != nil || len(inventory) != 2 {
		t.Fatal("restored management inventory", inventory, err)
	}
	// Restoration validates every payload hash. This fixture's disk providers and
	// app/root bridges are modeled; no guest filesystem or VM activation is proved.
}
