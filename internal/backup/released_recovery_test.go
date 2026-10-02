package backup

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestReleasedBackupRecoveryRequiresPublicationCompletionAndRootRelease(t *testing.T) {
	for _, scenario := range []string{"freezing", "active-worker", "retained-root", "released", "foreign-device", "missing-completion"} {
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
			if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
				t.Fatal(err)
			}
			if err = store.ClaimBackupLaunch(ctx, token, job.ID); err != nil {
				t.Fatal(err)
			}
			if scenario != "freezing" {
				if err = store.AttachMaintenanceRoot(ctx, token, job.ID, state.Random()); err != nil {
					t.Fatal(err)
				}
				if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
					t.Fatal(err)
				}
				if err = store.ClaimBackupPublication(ctx, token, job.ID, device); err != nil {
					t.Fatal(err)
				}
				if err = store.RecordBackupPublished(ctx, token, job.ID, device, state.Hash("snapshot")); err != nil {
					t.Fatal(err)
				}
				if scenario != "active-worker" {
					if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
						t.Fatal(err)
					}
					if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "publishing", "restoring"); err != nil {
						t.Fatal(err)
					}
					if scenario != "retained-root" {
						if err = store.ReleaseMaintenanceRoot(ctx, token, job.ID, func(context.Context, string) error { return nil }); err != nil {
							t.Fatal(err)
						}
						if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "restoring", "requires-action"); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if scenario == "missing-completion" {
				if _, err = store.DB.Exec("DELETE FROM settings WHERE key='host.backup-dispatch'"); err != nil {
					t.Fatal(err)
				}
			}
			apps := &coordinatorApps{}
			intended := device
			if scenario == "foreign-device" {
				intended = state.Random()
			}
			err = RecoverReleasedBackupMaintenance(ctx, store, token, job.ID, intended, apps)
			if scenario == "released" {
				if err != nil || !apps.restored {
					t.Fatal("released job not restored", err)
				}
			} else if err == nil || apps.restored {
				t.Fatal("unqualified recovery restored apps", scenario, err)
			}
		})
	}
}
