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
	for _, scenario := range []string{"success", "lost-launch", "repository-refused", "stopped-refusal", "refusal-restore-failed", "missing-publication", "lost-completion", "cleanup-failed", "cleanup-unrecorded"} {
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
			if scenario == "refusal-restore-failed" {
				apps.restoreErr = errors.New("fixture restore failure")
			}
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
				if scenario == "stopped-refusal" || scenario == "refusal-restore-failed" {
					return ErrLaunchRepositoryRefusedStopped
				}
				if scenario == "repository-refused" {
					return ErrLaunchRepositoryAdmission
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
			} else if scenario == "stopped-refusal" {
				if !errors.Is(err, ErrLaunchRepositoryRefusedStopped) || snapshot != "" || cleanupCalled || !apps.restored {
					t.Fatal("stopped refusal recovery failed", snapshot, err, cleanupCalled, apps.restored)
				}
				observation, e := store.InspectBackupObservation(ctx)
				if e != nil || observation.Current != nil || observation.LastPublished != nil || observation.WorkerCompletion != "none" {
					t.Fatal("refusal claimed publication", observation, e)
				}
				if _, _, e := store.BeginMaintenanceJob(ctx, device); e != nil {
					t.Fatal("refusal restoration retained admission", e)
				}
			} else if scenario == "refusal-restore-failed" {
				if !errors.Is(err, ErrLaunchRepositoryRefusedStopped) || !errors.Is(err, apps.restoreErr) || !apps.restored || cleanupCalled || snapshot != "" {
					t.Fatal("failed restoration misclassified", snapshot, err)
				}
				owned, e := store.InspectMaintenanceJob(ctx, token)
				if e != nil || owned.Phase != "requires-action" || owned.RootToken != "" {
					t.Fatal("failed refusal restoration lost recovery", owned, e)
				}
				if _, _, e := store.BeginMaintenanceJob(ctx, device); e == nil {
					t.Fatal("failed restoration reopened admission")
				}
				observation, e := store.InspectBackupObservation(ctx)
				if e != nil || observation.WorkerCompletion != "refused" || observation.Current != nil {
					t.Fatal(observation, e)
				}
			} else {
				if err == nil || apps.restored {
					t.Fatal("uncertain operation restored apps", err)
				}
				owned, e := store.InspectMaintenanceJob(ctx, token)
				expectedPhase := "requires-action"
				if scenario == "lost-launch" || scenario == "repository-refused" {
					expectedPhase = "freezing"
				}
				if e != nil || owned.Phase != expectedPhase {
					t.Fatal("recovery intent lost", owned, e)
				}
				if e := store.RequireBackupWorkerStopped(ctx, token, job.ID); (scenario == "lost-launch" || scenario == "repository-refused" || scenario == "missing-publication" || scenario == "lost-completion") && e == nil {
					t.Fatal("uncertain worker lost cleanup barrier")
				}
				if (scenario == "lost-launch" || scenario == "repository-refused" || scenario == "missing-publication" || scenario == "lost-completion") && cleanupCalled {
					t.Fatal("uncertain worker cleanup attempted")
				}
			}
			if _, err = credential.Stat(); err != nil {
				t.Fatal("caller credential closed", err)
			}
		})
	}
}
