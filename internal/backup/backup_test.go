package backup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type fixtureRecoveryPublisher struct {
	store   *state.Store
	root    *coordinatorRoot
	apps    *coordinatorApps
	calls   int
	failure error
}

func (p *fixtureRecoveryPublisher) Snapshot(ctx context.Context, directory *os.File, manifest Manifest, policy RestorePolicy) (string, error) {
	p.calls++
	if p.root.released || p.apps.restored {
		return "", errors.New("publication after cleanup")
	}
	var token string
	if err := p.store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='host.maintenance'").Scan(&token); err != nil {
		return "", err
	}
	job, err := p.store.InspectMaintenanceJob(ctx, token)
	if err != nil {
		return "", err
	}
	if job.Phase != "publishing" || job.RootToken != p.root.token {
		return "", state.ErrMaintenanceOwner
	}
	// The publisher receives the pinned staging directory with the complete,
	// validated management database and both disk payloads.
	if len(manifest.Files) != 3 {
		return "", ErrManifest
	}
	info, err := directory.Stat()
	if err != nil || !info.IsDir() {
		return "", ErrManifest
	}
	staged, err := os.OpenRoot(directory.Name())
	if err != nil {
		return "", err
	}
	defer staged.Close()
	if err = ValidateRecoverySet(ctx, staged, manifest, policy); err != nil {
		return "", err
	}
	if p.failure != nil {
		return "", p.failure
	}
	return state.Hash("fixture snapshot"), nil
}

func TestBackupOrchestrationStagesBeforePublicationAndRetainsCleanupFailure(t *testing.T) {
	for _, scenario := range []string{"success", "stage-failure", "publication-failure", "cleanup-failure"} {
		t.Run(scenario, func(t *testing.T) {
			store, disks, staging, token, policy := recoveryStageFixture(t)
			ctx := context.Background()
			if err := store.EndMaintenance(ctx, token); err != nil {
				t.Fatal(err)
			}
			device := state.Random()
			if _, err := store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
				t.Fatal(err)
			}
			apps := &coordinatorApps{}
			runtime := &coordinatorRoot{token: disks.token}
			publisher := &fixtureRecoveryPublisher{store: store, root: runtime, apps: apps}
			failure := errors.New("fixture failure")
			switch scenario {
			case "stage-failure":
				for id := range disks.instances {
					disks.failID = id
					break
				}
			case "publication-failure":
				publisher.failure = failure
			case "cleanup-failure":
				apps.restoreErr = failure
			}
			result, err := runBackup(ctx, store, device, apps, runtime, disks, staging, publisher, "0.1.0", 1, policy)
			if result.JobID == "" {
				t.Fatal("missing maintenance identity", err)
			}
			if scenario == "success" {
				if err != nil || result.SnapshotID == "" {
					t.Fatal(result, err)
				}
			} else if err == nil {
				t.Fatal("failure swallowed")
			}
			if scenario == "stage-failure" {
				if publisher.calls != 0 {
					t.Fatal("published failed staging")
				}
			} else if publisher.calls != 1 {
				t.Fatal("publication count", publisher.calls)
			}
			if scenario == "publication-failure" || scenario == "stage-failure" {
				if result.SnapshotID != "" {
					t.Fatal("failed backup claimed snapshot")
				}
			}
			if scenario == "cleanup-failure" {
				if result.SnapshotID == "" || !errors.Is(err, failure) {
					t.Fatal("published outcome lost", result, err)
				}
				if err = store.Transaction(ctx, state.RequireAdmission); !errors.Is(err, state.ErrMaintenance) {
					t.Fatal("failed cleanup reopened admission", err)
				}
			} else if err = store.Transaction(ctx, state.RequireAdmission); err != nil {
				t.Fatal("cleanup retained admission", err)
			}
		})
	}
}

func TestRunBackupCannotPublishUnqualifiedDisks(t *testing.T) {
	store, disks, staging, token, policy := recoveryStageFixture(t)
	ctx := context.Background()
	if err := store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
	device := state.Random()
	if _, err := store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	apps := &coordinatorApps{}
	runtime := &coordinatorRoot{token: disks.token}
	publisher := &fixtureRecoveryPublisher{store: store, root: runtime, apps: apps}
	result, err := RunBackup(ctx, store, device, apps, runtime, disks, staging, publisher, "0.1.0", 1, policy)
	if err == nil || result.JobID == "" || result.SnapshotID != "" || publisher.calls != 0 {
		t.Fatal("unqualified disk published", result, err, publisher.calls)
	}
	if !apps.restored || !runtime.released {
		t.Fatal("qualification failure skipped cleanup")
	}
	if err = store.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal("qualification failure retained admission", err)
	}
}
