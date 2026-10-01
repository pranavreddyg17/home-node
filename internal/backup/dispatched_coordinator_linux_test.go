//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDispatchedCoordinatorRequiresCompletionAndPublicationBeforeRelease(t *testing.T) {
	for _, scenario := range []string{"published", "delivery-failed", "missing-publication", "lost-completion", "admitted-published", "admitted-delivery-failed", "admitted-missing-publication", "admitted-lost-completion"} {
		t.Run(scenario, func(t *testing.T) {
			admitted := strings.HasPrefix(scenario, "admitted-")
			scenario = strings.TrimPrefix(scenario, "admitted-")
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
			credential, err := CreateRepositoryPassword([]byte("fixture-secret"))
			if err != nil {
				t.Fatal(err)
			}
			defer credential.Close()
			apps, root := &coordinatorApps{}, &coordinatorRoot{}
			var delivered Dispatch
			failure := errors.New("fixture lost delivery or completion")
			deliver := func(operation context.Context, d Dispatch, file *os.File) error {
				delivered = d
				if file != credential || root.released || apps.restored {
					t.Fatal("dispatch ownership/barriers invalid")
				}
				job, err := store.InspectMaintenanceJob(operation, d.ManagementToken)
				if err != nil || job.Phase != "staging" || job.RootToken != d.RuntimeToken {
					t.Fatal("dispatch not bound to frozen job", job, err)
				}
				if scenario == "delivery-failed" {
					return failure
				}
				if scenario == "missing-publication" {
					return nil
				}
				if err = store.AdvanceMaintenanceJob(operation, d.ManagementToken, d.JobID, "staging", "publishing"); err != nil {
					return err
				}
				if err = store.ClaimBackupPublication(operation, d.ManagementToken, d.JobID, d.DeviceID); err != nil {
					return err
				}
				if err = store.RecordBackupPublished(operation, d.ManagementToken, d.JobID, d.DeviceID, state.Hash("fixture snapshot")); err != nil {
					return err
				}
				if scenario == "lost-completion" {
					return failure
				}
				return nil
			}
			var jobID, snapshot string
			if admitted {
				token, job, admissionErr := store.BeginMaintenanceJob(ctx, device)
				if admissionErr != nil {
					t.Fatal(admissionErr)
				}
				for _, wrong := range []string{"token", "job", "device", "phase"} {
					owner, id, d := token, job.ID, device
					switch wrong {
					case "token":
						owner = state.Random()
					case "job":
						id = state.Random()
					case "device":
						d = state.Random()
					case "phase":
						if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
							t.Fatal(err)
						}
					}
					if _, _, denied := RunAdmittedDispatchedMaintenance(ctx, store, d, owner, id, "0.1.0", 1, credential, apps, root, deliver); denied == nil {
						t.Fatal("invalid admitted ownership accepted", wrong)
					}
					if delivered.JobID != "" || root.released || apps.restored {
						t.Fatal("invalid takeover changed barriers", wrong)
					}
					if wrong == "phase" {
						// Fixture repair only: production must reconcile an advanced job.
						if _, err = store.DB.Exec("UPDATE settings SET value='draining' WHERE key='host.maintenance-job.phase'"); err != nil {
							t.Fatal(err)
						}
					}
				}
				jobID, snapshot, err = RunAdmittedDispatchedMaintenance(ctx, store, device, token, job.ID, "0.1.0", 1, credential, apps, root, deliver)
				if jobID != job.ID {
					t.Fatal("admitted job replaced", jobID, job.ID)
				}
			} else {
				jobID, snapshot, err = RunDispatchedMaintenance(ctx, store, device, "0.1.0", 1, credential, apps, root, deliver)
			}
			if jobID == "" || delivered.JobID != jobID {
				t.Fatal("missing job binding")
			}
			if scenario == "published" {
				if err != nil || snapshot != state.Hash("fixture snapshot") || !root.released || !apps.restored {
					t.Fatal("completed publication not cleaned up", snapshot, err)
				}
			} else {
				if err == nil || snapshot != "" || root.released || apps.restored {
					t.Fatal("uncertain worker released barriers", scenario, snapshot, err)
				}
				job, inspectErr := store.InspectMaintenanceJob(ctx, delivered.ManagementToken)
				if inspectErr != nil || job.Phase != "requires-action" || job.RootToken != delivered.RuntimeToken {
					t.Fatal("uncertain ownership not retained", job, inspectErr)
				}
				if recoverErr := RecoverMaintenance(ctx, store, delivered.ManagementToken, jobID, apps, root); recoverErr == nil || root.released || apps.restored {
					t.Fatal("recovery released an uncertain worker", recoverErr)
				}
			}
			if _, err = credential.Stat(); err != nil {
				t.Fatal("caller credential closed", err)
			}
		})
	}
}
