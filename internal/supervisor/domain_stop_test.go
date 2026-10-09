package supervisor

import (
	"context"
	"errors"
	"testing"
)

type acknowledgedStopBackend struct{ *fakeBackend }

func (b *acknowledgedStopBackend) Stop(context.Context, string) error { b.stops++; return nil }

func TestStopAcknowledgementDoesNotCompleteTeardown(t *testing.T) {
	m, backend := newManager(t)
	ctx := context.Background()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	m.Backend = &acknowledgedStopBackend{backend}
	stop := start
	stop.Action, stop.OperationID, stop.Revision = "stop", startRequest().OperationID, 2
	if _, err := m.Apply(ctx, stop); !errors.Is(err, ErrPolicy) {
		t.Fatal("acknowledgement completed stop", err)
	}
	instance, err := m.Inspect(ctx, start.InstanceID)
	if err != nil || instance.State != "stopping" || instance.Desired != "stopped" {
		t.Fatal("uncertain stop lost", instance, err)
	}
	if err := m.Reconcile(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("reconciliation accepted running guest", err)
	}
	instance, err = m.Inspect(ctx, start.InstanceID)
	if err != nil || instance.State != "stopping" {
		t.Fatal("reconciliation falsely completed", instance, err)
	}
	m.Backend = backend
	if err := m.Reconcile(ctx); err != nil {
		t.Fatal("verified stop retry", err)
	}
	instance, err = m.Inspect(ctx, start.InstanceID)
	if err != nil || instance.State != "interrupted" {
		t.Fatal("verified stop not recorded", instance, err)
	}
}
