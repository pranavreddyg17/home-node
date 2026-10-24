package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestVideoRetirementRecoveryFromPersistedReceiptAfterUIDReuse(t *testing.T) {
	m, record := completedRetirementFixture(t)
	if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	replacement := state.Random()
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_uid_leases VALUES(?,?)`, replacement, record.Intent.Ownership.Channel.UID); err != nil {
		t.Fatal(err)
	}
	// A fresh manager has no in-memory intent, backend or pool. Only the durable
	// request/receipt is needed, never filesystem/process authority for the UID.
	if err := m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	m.Store = reopened
	fresh := &Manager{Store: reopened, Policy: m.Policy}
	for i := 0; i < 3; i++ {
		ok, err := fresh.completedVideoRetirement(context.Background(), record.Intent.Request)
		if err != nil || !ok {
			t.Fatal("receipt recovery", ok, err)
		}
		instance, err := fresh.Apply(context.Background(), record.Intent.Request)
		if err != nil || instance.State != "removed" || instance.ID != record.Intent.Request.InstanceID || instance.Revision != record.Intent.Request.Revision || instance.GuestUID != 0 {
			t.Fatal("public completion replay", instance, err)
		}

	}
	var uid uint32
	if err := m.Store.DB.QueryRow(`SELECT uid FROM runtime_uid_leases WHERE instance_id=?`, replacement).Scan(&uid); err != nil || uid != record.Intent.Ownership.Channel.UID {
		t.Fatal("replacement lease changed", uid, err)
	}
}

func TestVideoRetirementRecoveryPendingOrAbsentDoesNotClaimCompletion(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	if ok, err := m.completedVideoRetirement(context.Background(), r); err != nil || ok {
		t.Fatal("missing completion", ok, err)
	}
	record, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for {
		if ok, err := m.completedVideoRetirement(context.Background(), r); err != nil || ok {
			t.Fatal("pending completion", record.Stage, ok, err)
		}
		if record.Stage == "directory-removed" {
			break
		}
		record, err = m.advanceVideoRetirement(context.Background(), record, nextVideoRetirementStage(record.Stage), func(context.Context) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestVideoRetirementRecoveryRejectsChangedRequestsAndReceipts(t *testing.T) {
	for _, kind := range []string{"operation", "revision", "policy", "instance", "action", "unknown-field", "duplicate-key", "trailing-json", "oversized-intent", "oversized-phase", "orphan-intent", "missing-intent", "unknown-phase", "runtime-drift", "operation-drift"} {
		t.Run(kind, func(t *testing.T) {
			m, record := completedRetirementFixture(t)
			if err := m.commitVideoRetirement(context.Background(), record, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			r := record.Intent.Request
			key := videoRetirementKey(r.InstanceID)
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := m.Store.DB.Exec(query, args...); err != nil {
					t.Fatal(err)
				}
			}
			var saved string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "operation":
				r.OperationID = state.Random()
			case "revision":
				r.Revision++
			case "policy":
				r.PolicyGeneration++
			case "instance":
				r.InstanceID = "../foreign"
			case "action":
				r.Action = "start"
			case "unknown-field":
				exec(`UPDATE settings SET value=? WHERE key=?`, strings.TrimSuffix(saved, "}")+`,"extra":true}`, key)
			case "duplicate-key":
				exec(`UPDATE settings SET value=? WHERE key=?`, strings.TrimSuffix(saved, "}")+`,"schema":1}`, key)
			case "trailing-json":
				exec(`UPDATE settings SET value=? WHERE key=?`, saved+` {}`, key)
			case "oversized-intent":
				exec(`UPDATE settings SET value=? WHERE key=?`, strings.Repeat("x", 65537), key)
			case "oversized-phase":
				exec(`UPDATE settings SET value=? WHERE key=?`, strings.Repeat("x", 33), key+".stage")
			case "orphan-intent":
				exec(`DELETE FROM settings WHERE key=?`, key+".stage")
			case "missing-intent":
				exec(`DELETE FROM settings WHERE key=?`, key)
			case "unknown-phase":
				exec(`UPDATE settings SET value='unexpected' WHERE key=?`, key+".stage")
			case "runtime-drift":
				exec(`UPDATE runtime_instances SET revision=4 WHERE id=?`, r.InstanceID)
			case "operation-drift":
				exec(`UPDATE runtime_operations SET request_hash='foreign' WHERE id=?`, r.OperationID)
			}
			if ok, err := m.completedVideoRetirement(context.Background(), r); !errors.Is(err, ErrPolicy) || ok {
				t.Fatal("changed receipt admitted", ok, err)
			}
		})
	}
}
