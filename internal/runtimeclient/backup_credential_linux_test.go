//go:build linux

package runtimeclient

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestCredentialedWorkerClosesOwnedDescriptorOnRefusal(t *testing.T) {
	credential, err := backup.CreateRepositoryPassword([]byte("fixture-only-credential"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	result, err := RunCredentialedDispatchedBackup(context.Background(), backup.Dispatch{}, BackupWorkerConfig{}, credential)
	if err == nil || result != (backup.BackupResult{}) {
		t.Fatal("invalid credentialed job admitted")
	}
	if _, err = credential.Stat(); err == nil {
		t.Fatal("refusal retained credential descriptor")
	}
}

func TestLaunchedWorkerMissingDriveRefusesBeforeRuntimeAcquisition(t *testing.T) {
	credential, err := backup.CreateRepositoryPassword([]byte("fixture-only-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	parent := t.TempDir()
	launch := backup.Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	root := &acquisitionFixture{root: state.Random(), job: launch.JobID, token: launch.ManagementToken, device: launch.DeviceID}
	config := BackupWorkerConfig{ManagementSocket: filepath.Join(parent, "unused-management.sock"), DiskSocket: filepath.Join(parent, "unused-disk.sock"), StagingParent: parent, Release: "0.1.0", CatalogVersion: 1, Policy: backup.RestorePolicy{MinimumCatalogVersion: 1}, RepositoryTarget: backup.Target{MountPath: filepath.Join(parent, "missing-drive"), UUID: "fixture-missing-drive", RepositoryID: state.Hash("repository")}}
	dispatch, result, err := RunCredentialedLaunchedBackup(context.Background(), launch, config, credential, root)
	if !errors.Is(err, backup.ErrLaunchRepositoryAdmission) || !errors.Is(err, backup.ErrTarget) || dispatch != (backup.Dispatch{}) || result != (backup.BackupResult{}) || len(root.steps) != 0 {
		t.Fatal("unadmitted destination acquired runtime authority", dispatch, result, root.steps, err)
	}
	if _, err = credential.Stat(); err == nil {
		t.Fatal("missing-drive refusal retained credential")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("missing-drive refusal created staging", entries, err)
	}
}
