package backup

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestReleasedBackupRecoveryRequiresPublicationCompletionAndRootRelease(t *testing.T) {
	for _, scenario := range []string{"freezing", "active-worker", "retained-root", "released", "foreign-device", "missing-completion", "restore-failure", "restore-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			directory := filepath.Join(t.TempDir(), "management")
			store, err := state.Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { store.Close() }()
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
			var authorized string
			authorizationErr := store.Transaction(ctx, func(tx *sql.Tx) error {
				var err error
				authorized, _, err = state.AuthorizeReleasedBackupRecoveryTx(tx, device, job.ID)
				return err
			})
			eligible := scenario == "released" || scenario == "foreign-device" || scenario == "restore-failure" || scenario == "restore-cancel"
			if eligible && (authorizationErr != nil || authorized != token) {
				t.Fatal("eligible authority not bound", authorizationErr)
			}
			if !eligible && (authorizationErr == nil || authorized != "") {
				t.Fatal("unqualified authority exposed", scenario, authorizationErr)
			}
			if err := store.Transaction(ctx, func(tx *sql.Tx) error {
				_, _, err := state.AuthorizeReleasedBackupRecoveryTx(tx, state.Random(), job.ID)
				return err
			}); err == nil {
				t.Fatal("foreign recovery device authorized")
			}
			apps := &coordinatorApps{}
			intended := device
			if scenario == "foreign-device" {
				intended = state.Random()
			}
			if scenario == "restore-failure" {
				apps.restoreErr = errors.New("fixture app restart failed")
			}
			operation := ctx
			var restorationApps MaintenanceApps = apps
			if scenario == "restore-cancel" {
				var cancel context.CancelFunc
				operation, cancel = context.WithCancel(ctx)
				defer cancel()
				restorationApps = &cancelledRestorationApps{coordinatorApps: apps, cancel: cancel}
			}
			err = RecoverReleasedBackupMaintenance(operation, store, token, job.ID, intended, restorationApps)
			if scenario == "restore-failure" || scenario == "restore-cancel" {
				expected := apps.restoreErr
				if scenario == "restore-cancel" {
					expected = context.Canceled
				}
				if !errors.Is(err, expected) || !apps.restored {
					t.Fatal("restoration failure not retained", err)
				}
				retained, e := store.InspectMaintenanceJob(ctx, token)
				if e != nil || retained.Phase != "requires-action" || retained.RootToken != "" {
					t.Fatal("failed restoration checkpoint lost", retained, e)
				}
				if e = store.Transaction(ctx, state.RequireAdmission); e == nil {
					t.Fatal("failed restoration reopened admission")
				}
				if e = store.Close(); e != nil {
					t.Fatal(e)
				}
				store, e = state.Open(directory)
				if e != nil {
					t.Fatal(e)
				}
				retried := &coordinatorApps{}
				if e = RecoverReleasedBackupMaintenance(ctx, store, token, job.ID, device, retried); e != nil || !retried.restored {
					t.Fatal("restart recovery failed", e)
				}
				if e = store.Transaction(ctx, state.RequireAdmission); e != nil {
					t.Fatal("completed recovery kept admission closed", e)
				}
				return
			}
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

type cancelledRestorationApps struct {
	*coordinatorApps
	cancel context.CancelFunc
}

func (a *cancelledRestorationApps) RestoreMaintenanceApps(ctx context.Context, token, device string) error {
	a.cancel()
	a.coordinatorApps.RestoreMaintenanceApps(ctx, token, device)
	return ctx.Err()
}

func TestStoppedRepositoryRefusalRecoveryPreservesNoPublication(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "failed"}[fail], func(t *testing.T) {
			ctx := context.Background()
			store, err := state.Open(filepath.Join(t.TempDir(), "management"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			device := state.Random()
			if _, err := store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
				t.Fatal(err)
			}
			token, job, err := store.BeginMaintenanceJob(ctx, device)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
				t.Fatal(err)
			}
			if err := store.ClaimBackupLaunch(ctx, token, job.ID); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordBackupLaunchRefused(ctx, token, job.ID); err != nil {
				t.Fatal(err)
			}
			if err := store.AdvanceMaintenanceJob(ctx, token, job.ID, "restoring", "requires-action"); err != nil {
				t.Fatal(err)
			}
			// A contradictory publication claim must not qualify rootless refusal.
			if _, err := store.DB.Exec("INSERT INTO settings(key,value) VALUES('host.backup-publication',?)", job.ID); err != nil {
				t.Fatal(err)
			}
			blocked := &coordinatorApps{}
			if err := RecoverReleasedBackupMaintenance(ctx, store, token, job.ID, device, blocked); err == nil || blocked.restored {
				t.Fatal("contradictory refusal restored apps", err)
			}
			if _, err := store.DB.Exec("DELETE FROM settings WHERE key='host.backup-publication'"); err != nil {
				t.Fatal(err)
			}
			apps := &coordinatorApps{}
			if fail {
				apps.restoreErr = errors.New("fixture restoration failure")
			}
			err = RecoverReleasedBackupMaintenance(ctx, store, token, job.ID, device, apps)
			if !apps.restored || !apps.contextLive {
				t.Fatal("restoration not attempted")
			}
			if fail {
				if !errors.Is(err, apps.restoreErr) {
					t.Fatal(err)
				}
				owned, e := store.InspectMaintenanceJob(ctx, token)
				if e != nil || owned.Phase != "requires-action" || owned.RootToken != "" {
					t.Fatal(owned, e)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			observation, e := store.InspectBackupObservation(ctx)
			if e != nil || observation.Current != nil || observation.LastPublished != nil {
				t.Fatal("refusal invented publication", observation, e)
			}
			expected := "none"
			if fail {
				expected = "refused"
			}
			if observation.WorkerCompletion != expected {
				t.Fatal(observation)
			}
		})
	}
}
