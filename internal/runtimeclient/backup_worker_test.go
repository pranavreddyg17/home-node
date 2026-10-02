package runtimeclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestRegisteredWorkerRefusesDestinationBeforeCreatingStaging(t *testing.T) {
	parent := t.TempDir()
	dispatch := backup.Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	config := BackupWorkerConfig{ManagementSocket: "/tmp/unused-management.sock", DiskSocket: "/tmp/unused-disk.sock", StagingParent: parent, Release: "0.1.0", CatalogVersion: 1, Policy: backup.RestorePolicy{MinimumCatalogVersion: 1}, RepositoryTarget: backup.Target{MountPath: "relative-drive", UUID: "drive-fixture", RepositoryID: state.Hash("repository")}}
	for _, scenario := range []string{"target", "missing-drive", "release", "staging-parent", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			configured, message := config, dispatch
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "missing-drive":
				configured.RepositoryTarget.MountPath = filepath.Join(parent, "missing-drive")
			case "release":
				message.Release = "0.2.0"
			case "staging-parent":
				configured.StagingParent = "relative"
			case "cancelled":
				cancel()
			}
			result, err := RunRegisteredDispatchedBackup(ctx, message, configured, []byte("fixture-only-password"))
			if err == nil || result != (backup.BackupResult{}) {
				t.Fatal("invalid registered worker admitted", result, err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			files, err := os.ReadDir(parent)
			if err != nil || len(files) != 0 {
				t.Fatal("refused destination created staging", err)
			}
		})
	}
}

func TestLaunchedWorkerRefusesConfigurationBeforeRuntimeEffects(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "fixture-credential")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.WriteString("fixture-secret"); err != nil {
		t.Fatal(err)
	}
	launch := backup.Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	root := &acquisitionFixture{root: state.Random(), job: launch.JobID, token: launch.ManagementToken, device: launch.DeviceID}
	dispatch, result, err := RunCredentialedLaunchedBackup(context.Background(), launch, BackupWorkerConfig{}, file, root)
	if err == nil || dispatch != (backup.Dispatch{}) || result != (backup.BackupResult{}) || len(root.steps) != 0 {
		t.Fatal("invalid worker had effects", dispatch, result, err, root.steps)
	}
	if _, err = file.Stat(); err == nil {
		t.Fatal("refused launch retained owned credential")
	}
}

func TestRegisteredCleanupRejectsUnconfiguredManagementBeforeRootEffects(t *testing.T) {
	cleanup := backup.Cleanup{Version: 3, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random()}
	root := &releaseFixture{token: cleanup.ManagementToken, device: cleanup.DeviceID, root: cleanup.RuntimeToken}
	for _, socket := range []string{"", "relative", "/run/../run/management.sock"} {
		if err := RunRegisteredBackupCleanup(context.Background(), cleanup, BackupWorkerConfig{ManagementSocket: socket}, root); err == nil || len(root.steps) != 0 {
			t.Fatal("unconfigured cleanup reached root", err, root.steps)
		}
	}
}
