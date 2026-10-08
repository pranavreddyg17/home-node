package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStartFailureJournalUpdatesAreAtomic(t *testing.T) {
	m, backend := newManager(t)
	cause := errors.New("isolation refused")
	backend.verifyErr = cause
	r := startRequest()
	if _, err := m.Store.DB.Exec(`CREATE TRIGGER refuse_failed_operation BEFORE UPDATE OF state ON runtime_operations WHEN NEW.state='failed' BEGIN SELECT RAISE(ABORT,'fixture failure journal refused'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := m.Apply(context.Background(), r)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "fixture failure journal refused") {
		t.Fatal("failure cause or journal failure hidden", err)
	}
	if backend.running || backend.stops != 1 {
		t.Fatal("journal failure prevented teardown")
	}
	var phase, desired, operation string
	if err := m.Store.DB.QueryRow("SELECT state,desired FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&phase, &desired); err != nil {
		t.Fatal(err)
	}
	if phase != "preparing" || desired != "running" {
		t.Fatal("partial failed-start journal committed", phase, desired)
	}
	if err := m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if operation == "failed" {
		t.Fatal("refused operation mutation committed")
	}
}

func TestStartFailureKeepsUnconfirmedStopVisible(t *testing.T) {
	m, backend := newManager(t)
	cause := errors.New("isolation refused")
	backend.verifyErr = cause
	r := startRequest()
	m.Backend = &failingStopBackend{fakeBackend: backend, failID: r.InstanceID}
	_, err := m.Apply(context.Background(), r)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "stop failed") {
		t.Fatal("stop failure hidden", err)
	}
	if !backend.running {
		t.Fatal("failed stop reported absent")
	}
	var phase, desired string
	if err := m.Store.DB.QueryRow("SELECT state,desired FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&phase, &desired); err != nil {
		t.Fatal(err)
	}
	if phase != "stopping" || desired != "stopped" {
		t.Fatal("unconfirmed teardown lost", phase, desired)
	}
}
