package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// SQL-only fixture: advancing these checkpoints is not physical absence proof.
func completedRetirementFixture(t *testing.T) (*Manager, videoRetirementRecord) {
	t.Helper()
	m, r := retirementSnapshotFixture(t)
	guard := func(context.Context) error { return nil }
	record, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil {
		t.Fatal(err)
	}
	for record.Stage != "directory-removed" {
		record, err = m.advanceVideoRetirement(context.Background(), record, nextVideoRetirementStage(record.Stage), guard)
		if err != nil {
			t.Fatal(err)
		}
	}
	return m, record
}

func TestVideoRetirementCompletionAtomicAndReplayAfterUIDReuse(t *testing.T) {
	m, record := completedRetirementFixture(t)
	calls := 0
	guard := func(context.Context) error { calls++; return nil }
	if err := m.commitVideoRetirement(context.Background(), record, guard); err != nil {
		t.Fatal(err)
	}
	replacement := state.Random()
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_uid_leases(instance_id,uid) VALUES(?,?)`, replacement, record.Intent.Ownership.Channel.UID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { t.Fatal("completed replay checked reused resources"); return ErrPolicy }); err != nil {
			t.Fatal(err)
		}
	}
	var uid uint32
	if err := m.Store.DB.QueryRow(`SELECT uid FROM runtime_uid_leases WHERE instance_id=?`, replacement).Scan(&uid); err != nil || uid != record.Intent.Ownership.Channel.UID || calls != 1 {
		t.Fatal("replay affected reused UID", uid, calls, err)
	}
}

func TestVideoRetirementCompletionRollsBackEveryOwnershipRelease(t *testing.T) {
	m, record := completedRetirementFixture(t)
	if _, err := m.Store.DB.Exec(`CREATE TRIGGER refuse_retirement_receipt BEFORE UPDATE OF value ON settings WHEN NEW.value='completed' BEGIN SELECT RAISE(ABORT,'fixture completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { return nil }); err == nil {
		t.Fatal("injected completion failure absent")
	}
	if err := m.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		phase, err := m.readVideoRetirementRecordTx(context.Background(), tx, record.Intent)
		if err == nil && phase != "directory-removed" {
			return ErrPolicy
		}
		return err
	}); err != nil {
		t.Fatal("failure partially released ownership", err)
	}
	if _, err := m.Store.DB.Exec(`DROP TRIGGER refuse_retirement_receipt`); err != nil {
		t.Fatal(err)
	}
	if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { return nil }); err != nil {
		t.Fatal("retry failed", err)
	}
}

func TestVideoRetirementCompletionRefusesLostAuthority(t *testing.T) {
	for _, kind := range []string{"guard", "phase", "intent", "pending-operation", "maintenance"} {
		t.Run(kind, func(t *testing.T) {
			m, record := completedRetirementFixture(t)
			guard := func(context.Context) error { return nil }
			switch kind {
			case "guard":
				guard = func(context.Context) error { return ErrPolicy }
			case "phase":
				record.Stage = "volume-removed"
			case "intent":
				record.Intent.Ownership.Volume.Inode++
			case "pending-operation":
				if _, err := m.Store.DB.Exec(`UPDATE runtime_operations SET state='succeeded' WHERE id=?`, record.Intent.Request.OperationID); err != nil {
					t.Fatal(err)
				}
			case "maintenance":
				if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES('runtime.maintenance','fixture')`); err != nil {
					t.Fatal(err)
				}
			}
			if err := m.commitVideoRetirement(context.Background(), record, guard); !errors.Is(err, ErrPolicy) {
				t.Fatal("lost authority admitted", err)
			}
			var leases int
			if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
				t.Fatal("refusal released lease", leases, err)
			}
		})
	}
}

func TestVideoRetirementCompletedReceiptRefusesTampering(t *testing.T) {
	for _, query := range []string{`UPDATE runtime_operations SET request_hash='changed'`, `UPDATE runtime_instances SET state='stopped'`, `UPDATE runtime_instances SET revision=4`, `UPDATE settings SET value='{}' WHERE key NOT LIKE '%.stage' AND key LIKE 'runtime.video-retirement.%'`} {
		t.Run(query, func(t *testing.T) {
			m, record := completedRetirementFixture(t)
			if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Store.DB.Exec(query); err != nil {
				t.Fatal(err)
			}
			if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { t.Fatal("tampered completed receipt invoked absence guard"); return nil }); !errors.Is(err, ErrPolicy) {
				t.Fatal("tampered receipt admitted", err)
			}
		})
	}
}
