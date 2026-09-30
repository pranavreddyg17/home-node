package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRuntimeMaintenanceReleaseReceiptSurvivesReopenAndPreservesNewBarrier(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	old, err := m.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.EndRuntimeMaintenance(ctx, old); err != nil {
		t.Fatal(err)
	}
	var receipt string
	if err = m.Store.DB.QueryRow("SELECT value FROM settings WHERE key='runtime.maintenance-release'").Scan(&receipt); err != nil || receipt != state.Hash(old) {
		t.Fatal("raw or missing receipt", receipt, err)
	}
	if err = m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	m.Store = reopened
	replacement := &Manager{Store: m.Store, Backend: m.Backend, Policy: m.Policy, Manifest: m.Manifest, Images: m.Images, Volumes: m.Volumes, Channels: m.Channels}
	if err = replacement.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := replacement.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = replacement.EndRuntimeMaintenance(ctx, old); err != nil {
		t.Fatal("lost response retry refused", err)
	}
	var owner string
	if err = m.Store.DB.QueryRow("SELECT value FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&owner); err != nil || owner != current {
		t.Fatal("old retry released newer barrier", owner, err)
	}
	if err = replacement.EndRuntimeMaintenance(ctx, state.Random()); !errors.Is(err, ErrPolicy) {
		t.Fatal("unowned token acknowledged", err)
	}
	if err = replacement.EndRuntimeMaintenance(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err = replacement.EndRuntimeMaintenance(ctx, old); !errors.Is(err, ErrPolicy) {
		t.Fatal("superseded receipt accepted", err)
	}
}

func TestRuntimeMaintenanceReleaseRollsBackIfReceiptWriteFails(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	token, err := m.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Store.DB.Exec("CREATE TRIGGER fixture_receipt BEFORE INSERT ON settings WHEN NEW.key='runtime.maintenance-release' BEGIN SELECT RAISE(ABORT,'fixture receipt failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err = m.EndRuntimeMaintenance(ctx, token); err == nil {
		t.Fatal("receipt failure hidden")
	}
	var owner string
	if err = m.Store.DB.QueryRow("SELECT value FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&owner); err != nil || owner != token {
		t.Fatal("barrier deleted without receipt", owner, err)
	}
}

func TestRootReleaseLostAcknowledgementCanResumeManagementCheckpoint(t *testing.T) {
	m, _ := newManager(t)
	management, err := state.Open(filepath.Join(t.TempDir(), "management"))
	if err != nil {
		t.Fatal(err)
	}
	defer management.Close()
	ctx := context.Background()
	device := state.Random()
	if _, err = management.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','[\"admin\"]',1)", device); err != nil {
		t.Fatal(err)
	}
	token, job, err := management.BeginMaintenanceJob(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if err = management.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		t.Fatal(err)
	}
	rootToken, err := m.BeginRuntimeMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = management.AttachMaintenanceRoot(ctx, token, job.ID, rootToken); err != nil {
		t.Fatal(err)
	}
	if err = management.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "restoring"); err != nil {
		t.Fatal(err)
	}
	lost := errors.New("fixture acknowledgement lost after root commit")
	err = management.ReleaseMaintenanceRoot(ctx, token, job.ID, func(ctx context.Context, token string) error {
		if err := m.EndRuntimeMaintenance(ctx, token); err != nil {
			return err
		}
		return lost
	})
	if !errors.Is(err, lost) {
		t.Fatal(err)
	}
	retained, err := management.InspectMaintenanceJob(ctx, token)
	if err != nil || retained.RootToken != rootToken {
		t.Fatal("uncertain checkpoint lost authority", retained, err)
	}
	if err = management.ReleaseMaintenanceRoot(ctx, token, job.ID, m.EndRuntimeMaintenance); err != nil {
		t.Fatal("receipt failed to reconcile checkpoint", err)
	}
	if err = management.CompleteMaintenanceJob(ctx, token, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = management.Transaction(ctx, state.RequireAdmission); err != nil {
		t.Fatal(err)
	}
}
