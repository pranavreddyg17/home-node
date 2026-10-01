package runtimeclient

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type workerRepositoryFixture struct{ calls int }

func (r *workerRepositoryFixture) Snapshot(context.Context, *os.File, backup.Manifest, backup.RestorePolicy) (string, error) {
	r.calls++
	return "", errors.New("unexpected repository write")
}
func TestBackupWorkerRejectsUninstalledDispatchBeforeEffects(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dispatch := backup.Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 2}
	config := BackupWorkerConfig{ManagementSocket: "/tmp/unused-management.sock", DiskSocket: "/tmp/unused-disk.sock", Release: "0.1.0", StagingParent: root.Name(), CatalogVersion: 2, Policy: backup.RestorePolicy{MinimumCatalogVersion: 1}}
	for _, scenario := range []string{"release", "catalog", "floor", "management-path", "disk-path", "invalid-job", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			configured, message := config, dispatch
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "release":
				message.Release = "0.2.0"
			case "catalog":
				message.CatalogVersion = 1
			case "floor":
				configured.Policy.MinimumCatalogVersion = 3
			case "management-path":
				configured.ManagementSocket = "relative.sock"
			case "disk-path":
				configured.DiskSocket = "/tmp/../tmp/disk.sock"
			case "invalid-job":
				message.JobID = "short"
			case "cancelled":
				cancel()
			}
			repository := &workerRepositoryFixture{}
			result, err := RunDispatchedBackup(ctx, message, configured, root, repository)
			if err == nil || result != (backup.BackupResult{}) || repository.calls != 0 {
				t.Fatal("invalid dispatch reached worker effects", result, err)
			}
			leasedResult, leasedErr := RunLeasedDispatchedBackup(ctx, message, configured, repository)
			if leasedErr == nil || leasedResult != (backup.BackupResult{}) || repository.calls != 0 {
				t.Fatal("invalid leased dispatch reached effects", leasedResult, leasedErr)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			files, err := root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			names, readErr := files.Readdirnames(-1)
			files.Close()
			if readErr != nil || len(names) != 0 {
				t.Fatal("refused worker wrote staging", readErr, names)
			}
		})
	}
}
