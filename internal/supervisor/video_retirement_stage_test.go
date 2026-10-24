package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestVideoRetirementStagesRequireExactForwardEdgesAndReplay(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	guard := func(context.Context) error { return nil }
	record, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"socket-removing", "socket-removed", "volume-removing", "volume-removed", "directory-removing", "directory-removed"} {
		next, err := m.advanceVideoRetirement(context.Background(), record, stage, guard)
		if err != nil || next.Stage != stage {
			t.Fatal("forward checkpoint", next, err)
		}
		replay, err := m.advanceVideoRetirement(context.Background(), record, stage, guard)
		if err != nil || replay.Stage != stage {
			t.Fatal("checkpoint replay", replay, err)
		}
		record = next
	}
	if got, err := m.advanceVideoRetirement(context.Background(), record, "complete", guard); !errors.Is(err, ErrPolicy) || got.Stage != "" {
		t.Fatal("checkpoint released ownership", got, err)
	}
	var leases int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, r.InstanceID).Scan(&leases); err != nil || leases != 1 {
		t.Fatal("checkpoints released UID", leases, err)
	}
}

func TestVideoRetirementStagesRefuseSkippedAndChangedAuthority(t *testing.T) {
	for _, scenario := range []string{"skip", "backwards", "inode", "operation", "intent", "stage"} {
		t.Run(scenario, func(t *testing.T) {
			m, r := retirementSnapshotFixture(t)
			guard := func(context.Context) error { return nil }
			record, err := m.prepareVideoRetirement(context.Background(), r, guard)
			if err != nil {
				t.Fatal(err)
			}
			target := "socket-removing"
			switch scenario {
			case "skip":
				target = "volume-removing"
			case "backwards":
				target = "prepared"
			case "inode":
				record.Intent.Ownership.Volume.Inode++
			case "operation":
				if _, err := m.Store.DB.Exec(`DELETE FROM runtime_operations WHERE id=?`, r.OperationID); err != nil {
					t.Fatal(err)
				}
			case "intent":
				if _, err := m.Store.DB.Exec(`UPDATE settings SET value='{}' WHERE key=?`, videoRetirementKey(r.InstanceID)); err != nil {
					t.Fatal(err)
				}
			case "stage":
				if _, err := m.Store.DB.Exec(`UPDATE settings SET value='unknown' WHERE key=?`, videoRetirementKey(r.InstanceID)+".stage"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := m.advanceVideoRetirement(context.Background(), record, target, guard)
			if !errors.Is(err, ErrPolicy) || got.Stage != "" || got.Intent.Ownership.InstanceID != "" {
				t.Fatal("invalid checkpoint returned authority", got, err)
			}
		})
	}
}

func TestVideoRetirementCheckpointLostGuardPreservesResumableStage(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	record, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	lost := errors.New("descriptor scope lost")
	got, err := m.advanceVideoRetirement(context.Background(), record, "socket-removing", func(context.Context) error {
		calls++
		if calls == 2 {
			return lost
		}
		return nil
	})
	if !errors.Is(err, lost) || got.Stage != "" || got.Intent.Ownership.InstanceID != "" {
		t.Fatal("lost guard returned receipt", got, err)
	}
	retry, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil })
	if err != nil || retry.Stage != "socket-removing" {
		t.Fatal("checkpoint intent not resumable", retry, err)
	}
}

func TestVideoRetirementCheckpointFailureDoesNotAdvanceStage(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	guard := func(context.Context) error { return nil }
	record, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`CREATE TRIGGER deny_retirement_checkpoint BEFORE UPDATE ON settings WHEN NEW.key LIKE 'runtime.video-retirement.%.stage' BEGIN SELECT RAISE(ABORT,'fixture interruption'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := m.advanceVideoRetirement(context.Background(), record, "socket-removing", guard); err == nil || got.Stage != "" {
		t.Fatal("failed update returned receipt", got, err)
	}
	retry, err := m.prepareVideoRetirement(context.Background(), r, guard)
	if err != nil || retry.Stage != "prepared" {
		t.Fatal("failed checkpoint changed phase", retry, err)
	}
}
