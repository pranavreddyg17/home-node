//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestLaunchedCoordinatorQualifiesWorkerBeforeCleanupAndRestore(t *testing.T) {
	for _, scenario := range []string{"success", "lost-launch", "missing-publication", "lost-completion", "cleanup-failed", "cleanup-unrecorded"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store, err := state.Open(filepath.Join(t.TempDir(), "management"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			device := state.Random()
			if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
				t.Fatal(err)
			}
			token, job, err := store.BeginMaintenanceJob(ctx, device)
			if err != nil {
				t.Fatal(err)
			}
			credential, err := CreateRepositoryPassword([]byte("fixture-secret"))
			if err != nil {
				t.Fatal(err)
			}
			defer credential.Close()
			apps := &coordinatorApps{}
			rootToken := state.Random()
			cleanupCalled := false
			failure := errors.New("fixture uncertain transport")
			launch := func(operation context.Context, l Launch, file *os.File) error {
				if l.JobID != job.ID || l.ManagementToken != token || file != credential || apps.restored {
					t.Fatal("incorrect launch binding")
				}
				owned, e := store.InspectMaintenanceJob(operation, token)
				if e != nil || owned.Phase != "freezing" || owned.RootToken != "" {
					t.Fatal(owned, e)
				}
				if e = store.RequireBackupWorkerStopped(operation, token, job.ID); e == nil {
					t.Fatal("handoff not durably claimed")
				}
				if scenario == "lost-launch" {
					return failure
				}
				if e = store.AttachMaintenanceRoot(operation, token, job.ID, rootToken); e != nil {
					return e
				}
				if scenario == "missing-publication" {
					return nil
				}
				if e = store.AdvanceMaintenanceJob(operation, token, job.ID, "staging", "publishing"); e != nil {
					return e
				}
				if e = store.ClaimBackupPublication(operation, token, job.ID, device); e != nil {
					return e
				}
				if e = store.RecordBackupPublished(operation, token, job.ID, device, state.Hash("snapshot")); e != nil {
					return e
				}
				if scenario == "lost-completion" {
					return failure
				}
				return nil
			}
			cleanup := func(operation context.Context, c Cleanup, file *os.File) error {
				cleanupCalled = true
				if c.JobID != job.ID || c.RuntimeToken != rootToken || c.ManagementToken != token || file != credential || apps.restored {
					t.Fatal("incorrect cleanup binding")
				}
				if e := store.RequireBackupWorkerStopped(operation, token, job.ID); e != nil {
					t.Fatal(e)
				}
				if scenario == "cleanup-failed" {
					return failure
				}
				if scenario == "cleanup-unrecorded" {
					return nil
				}
				return store.ReleaseMaintenanceRoot(operation, token, job.ID, func(context.Context, string) error { return nil })
			}
			snapshot, err := RunAdmittedLaunchedMaintenance(ctx, store, device, token, job.ID, "0.1.0", 1, credential, apps, launch, cleanup)
			if scenario == "success" {
				if err != nil || snapshot != state.Hash("snapshot") || !cleanupCalled || !apps.restored {
					t.Fatal(snapshot, err, cleanupCalled, apps.restored)
				}
			} else {
				if err == nil || apps.restored {
					t.Fatal("uncertain operation restored apps", err)
				}
				owned, e := store.InspectMaintenanceJob(ctx, token)
				if e != nil || owned.Phase != "requires-action" {
					t.Fatal("recovery intent lost", owned, e)
				}
				if (scenario == "lost-launch" || scenario == "missing-publication" || scenario == "lost-completion") && cleanupCalled {
					t.Fatal("uncertain worker cleanup attempted")
				}
			}
			if _, err = credential.Stat(); err != nil {
				t.Fatal("caller credential closed", err)
			}
		})
	}
}
