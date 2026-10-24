package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestVideoRetirementIntentAtomicReplayAndStoppedBarrier(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	calls := 0
	guard := func(context.Context) error { calls++; return nil }
	got, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil || got.Stage != "prepared" || got.Intent.Request != r || got.Intent.Ownership.Volume.Inode != 100 || calls != 2 {
		t.Fatal(got, calls, err)
	}
	again, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil || again.Stage != "prepared" || again.Intent.Ownership.Volume != got.Intent.Ownership.Volume {
		t.Fatal("replay lost original ownership", again, err)
	}
	var operations, leases int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_operations WHERE id=?`, r.OperationID).Scan(&operations); err != nil || operations != 1 {
		t.Fatal(operations, err)
	}
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, r.InstanceID).Scan(&leases); err != nil || leases != 1 {
		t.Fatal("preparation released UID", leases, err)
	}
	var phase string
	if err := m.Store.DB.QueryRow(`SELECT state FROM runtime_instances WHERE id=?`, r.InstanceID).Scan(&phase); err != nil || phase != "stopped" {
		t.Fatal("preparation changed runtime", phase, err)
	}
}

func TestVideoRetirementIntentRefusesChangedBindingOrUnknownStage(t *testing.T) {
	for _, scenario := range []string{"request", "inode", "stage", "missing-stage", "orphan-operation"} {
		t.Run(scenario, func(t *testing.T) {
			m, r := retirementSnapshotFixture(t)
			guard := func(context.Context) error { return nil }
			if scenario == "orphan-operation" {
				if err := m.Store.Transaction(context.Background(), func(tx *sql.Tx) error { _, err := m.operation(tx, r); return err }); err != nil {
					t.Fatal(err)
				}
			} else if _, err := m.prepareVideoRetirement(context.Background(), r, guard); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "request":
				r.OperationID = state.Random()
			case "inode":
				if _, err := m.Store.DB.Exec(`UPDATE runtime_volume_ownership SET inode=101`); err != nil {
					t.Fatal(err)
				}
			case "stage":
				if _, err := m.Store.DB.Exec(`UPDATE settings SET value='unknown' WHERE key=?`, videoRetirementKey(r.InstanceID)+".stage"); err != nil {
					t.Fatal(err)
				}
			case "missing-stage":
				if _, err := m.Store.DB.Exec(`DELETE FROM settings WHERE key=?`, videoRetirementKey(r.InstanceID)+".stage"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := m.prepareVideoRetirement(context.Background(), r, guard)
			if !errors.Is(err, ErrPolicy) || got.Stage != "" || got.Intent.Ownership.InstanceID != "" {
				t.Fatal("inconsistent intent authorized", got, err)
			}
		})
	}
}

func TestVideoRetirementIntentWriteFailureRollsBackOperation(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	if _, err := m.Store.DB.Exec(`CREATE TRIGGER deny_retirement_stage BEFORE INSERT ON settings WHEN NEW.key LIKE 'runtime.video-retirement.%.stage' BEGIN SELECT RAISE(ABORT,'fixture interruption'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil }); err == nil || got.Stage != "" {
		t.Fatal("failed preparation returned authority", got, err)
	}
	var count int
	if err := m.Store.DB.QueryRow(`SELECT (SELECT count(*) FROM runtime_operations WHERE id=?)+(SELECT count(*) FROM settings WHERE key IN(?,?))`, r.OperationID, videoRetirementKey(r.InstanceID), videoRetirementKey(r.InstanceID)+".stage").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial intent survived failed transaction", count, err)
	}
}

func TestVideoRetirementIntentLostBarrierRetainsOnlyPendingIntent(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	lost := errors.New("stopped scope lost")
	calls := 0
	got, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error {
		calls++
		if calls == 2 {
			return lost
		}
		return nil
	})
	if !errors.Is(err, lost) || got.Stage != "" || got.Intent.Ownership.InstanceID != "" {
		t.Fatal("lost barrier returned authority", got, err)
	}
	var stage string
	if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(r.InstanceID)+".stage").Scan(&stage); err != nil || stage != "prepared" {
		t.Fatal("pending intent lost", stage, err)
	}
	if _, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil }); err != nil {
		t.Fatal("pending replay", err)
	}
}
