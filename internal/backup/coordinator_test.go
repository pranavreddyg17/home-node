package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type coordinatorApps struct {
	restoreErr  error
	restored    bool
	contextLive bool
}

func (*coordinatorApps) DrainMaintenance(context.Context, string, string) error { return nil }
func (a *coordinatorApps) RestoreMaintenanceApps(ctx context.Context, _ string, _ string) error {
	a.restored = true
	a.contextLive = ctx.Err() == nil
	return a.restoreErr
}

type coordinatorRoot struct {
	token      string
	released   bool
	lost       bool
	acquires   int
	releaseErr error
	acquireErr error
}

func (r *coordinatorRoot) BeginRuntimeMaintenanceForJob(context.Context, string) (string, error) {
	r.acquires++
	if r.token == "" {
		r.token = state.Random()
	}
	if r.acquireErr != nil {
		return "", r.acquireErr
	}
	if r.lost {
		r.lost = false
		return "", errors.New("fixture acquisition response lost")
	}
	return r.token, nil
}
func (r *coordinatorRoot) EndRuntimeMaintenance(ctx context.Context, token string) error {
	if ctx.Err() != nil || token != r.token {
		return ErrManifest
	}
	if r.releaseErr != nil {
		return r.releaseErr
	}
	r.released = true
	return nil
}

func TestMaintenanceCoordinatorCleansUpWorkFailureAndCancellation(t *testing.T) {
	for _, scenario := range []string{"success", "stage-failure", "publish-cancel", "acquisition-lost", "restore-failure", "release-failure"} {
		t.Run(scenario, func(t *testing.T) {
			store, err := state.Open(filepath.Join(t.TempDir(), "management"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			device := state.Random()
			if _, err = store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
				t.Fatal(err)
			}
			apps := &coordinatorApps{}
			root := &coordinatorRoot{lost: scenario == "acquisition-lost"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			workError := errors.New("fixture stage failed")
			if scenario == "restore-failure" {
				apps.restoreErr = workError
			}
			if scenario == "release-failure" {
				root.releaseErr = workError
			}
			stage := func(ctx context.Context, token, rootToken string) error {
				job, err := store.InspectMaintenanceJob(ctx, token)
				if err != nil || job.Phase != "staging" || job.RootToken != rootToken {
					t.Fatal(job, err)
				}
				if scenario == "stage-failure" {
					return workError
				}
				return nil
			}
			publish := func(ctx context.Context, token, rootToken string) error {
				job, err := store.InspectMaintenanceJob(ctx, token)
				if err != nil || job.Phase != "publishing" || job.RootToken != rootToken {
					t.Fatal(job, err)
				}
				if scenario == "publish-cancel" {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			id, err := RunMaintenance(ctx, store, device, apps, root, stage, publish)
			if id == "" || scenario == "success" && err != nil || scenario != "success" && err == nil {
				t.Fatal(id, err)
			}
			if scenario != "release-failure" && (!root.released || !apps.restored || !apps.contextLive) {
				t.Fatal("cleanup skipped or used cancelled work context", root, apps)
			}
			admissionErr := store.Transaction(context.Background(), state.RequireAdmission)
			if scenario == "restore-failure" || scenario == "release-failure" {
				if !errors.Is(admissionErr, state.ErrMaintenance) {
					t.Fatal("failed restoration reopened admission", admissionErr)
				}
				var phase string
				if err := store.DB.QueryRow("SELECT value FROM settings WHERE key='host.maintenance-job.phase'").Scan(&phase); err != nil || phase != "requires-action" {
					t.Fatal(phase, err)
				}
			} else if admissionErr != nil {
				t.Fatal("successful cleanup retained barrier", admissionErr)
			}
			if scenario == "acquisition-lost" && root.acquires != 2 {
				t.Fatal("uncertain acquisition not reconciled", root.acquires)
			}
		})
	}
}
