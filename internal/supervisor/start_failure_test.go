package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
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
	if _, err := m.Store.DB.Exec("DROP TRIGGER refuse_failed_operation"); err != nil {
		t.Fatal(err)
	}
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal("failure journal reconciliation", err)
	}
	if err := m.Store.DB.QueryRow("SELECT state,desired FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&phase, &desired); err != nil {
		t.Fatal(err)
	}
	if err := m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if phase != "interrupted" || desired != "stopped" || operation != "interrupted" || backend.running || backend.starts != 1 {
		t.Fatal("uncertain start did not reconcile without relaunch", phase, desired, operation)
	}
}

func TestCancelledStartPreservesNewerConfirmedStop(t *testing.T) {
	m, backend := newManager(t)
	ctx := context.Background()
	r := startRequest()
	m.Backend = &changingAuditBackend{fakeBackend: backend, change: func() error {
		stop := r
		stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), 2
		_, err := m.Apply(ctx, stop)
		return errors.Join(errors.New("launch superseded by stop"), err)
	}}
	if _, err := m.Apply(ctx, r); err == nil {
		t.Fatal("superseded launch succeeded")
	}
	i, err := m.Inspect(ctx, r.InstanceID)
	if err != nil || i.Revision != 2 || i.State != "stopped" || i.Desired != "stopped" {
		t.Fatal("old launch overwrote confirmed stop", i, err)
	}
	if backend.running {
		t.Fatal("superseded guest remained running")
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
	m.Backend = backend
	if err := m.Audit(context.Background()); err != nil {
		t.Fatal("failed-start stop retry", err)
	}
	if err := m.Store.DB.QueryRow("SELECT state,desired FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&phase, &desired); err != nil {
		t.Fatal(err)
	}
	if phase != "interrupted" || desired != "stopped" || backend.running || backend.starts != 1 || backend.stops != 2 {
		t.Fatal("audit lost failed-start teardown", phase, desired, backend.starts, backend.stops)
	}
}

type cancelledStartVerifyBackend struct {
	*fakeBackend
	cancel context.CancelFunc
}

type supersededCleanupBackend struct {
	*fakeBackend
	verify         func() error
	cleanupFailure error
}

func (b *supersededCleanupBackend) Verify(context.Context, Domain) error { return b.verify() }
func (b *supersededCleanupBackend) Stop(ctx context.Context, id string) error {
	if b.stops != 0 {
		return b.cleanupFailure
	}
	return b.fakeBackend.Stop(ctx, id)
}

func TestSupersededLaunchCleanupFailureRemainsAuditable(t *testing.T) {
	m, original := newManager(t)
	ctx := context.Background()
	r := startRequest()
	failure := errors.New("cleanup teardown unconfirmed")
	b := &supersededCleanupBackend{fakeBackend: original, cleanupFailure: failure}
	b.verify = func() error {
		stop := r
		stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), 2
		_, err := m.Apply(ctx, stop)
		return errors.Join(errors.New("launch superseded"), err)
	}
	m.Backend = b
	if _, err := m.Apply(ctx, r); !errors.Is(err, failure) {
		t.Fatal("cleanup uncertainty hidden", err)
	}
	i, err := m.Inspect(ctx, r.InstanceID)
	if err != nil || i.Revision != 2 || i.State != "stopping" || i.Desired != "stopped" {
		t.Fatal("cleanup uncertainty lost", i, err)
	}
	m.Backend = original
	if err := m.Audit(ctx); err != nil {
		t.Fatal(err)
	}
	i, err = m.Inspect(ctx, r.InstanceID)
	if err != nil || i.Revision != 2 || i.State != "interrupted" || original.stops != 2 {
		t.Fatal("audit skipped uncertain teardown", i, err, original.stops)
	}
}

func (b *cancelledStartVerifyBackend) Verify(context.Context, Domain) error {
	b.cancel()
	return context.Canceled
}
func TestStartFailureJournalsAfterCallerCancellation(t *testing.T) {
	m, backend := newManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Backend = &cancelledStartVerifyBackend{fakeBackend: backend, cancel: cancel}
	r := startRequest()
	if _, err := m.Apply(ctx, r); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	if backend.running || backend.stops != 1 {
		t.Fatal("cancelled caller prevented teardown")
	}
	var phase, desired, operation string
	if err := m.Store.DB.QueryRow("SELECT state,desired FROM runtime_instances WHERE id=?", r.InstanceID).Scan(&phase, &desired); err != nil {
		t.Fatal(err)
	}
	if err := m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if phase != "failed" || desired != "stopped" || operation != "failed" {
		t.Fatal("cancelled caller prevented failure journal", phase, desired, operation)
	}
}
