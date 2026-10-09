package supervisor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type overlappingStopBackend struct {
	*fakeBackend
	calls             atomic.Int32
	entered           chan struct{}
	release           chan struct{}
	validationEntered chan struct{}
	validationRelease chan struct{}
}

func (b *overlappingStopBackend) ValidateHost(ctx context.Context, p Policy) error {
	if b.validationEntered != nil {
		close(b.validationEntered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.validationRelease:
		}
	}
	return b.fakeBackend.ValidateHost(ctx, p)
}

func (b *overlappingStopBackend) Stop(ctx context.Context, id string) error {
	if b.calls.Add(1) == 1 {
		close(b.entered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.release:
		}
	}
	return b.fakeBackend.Stop(ctx, id)
}

func TestStopDuringHostValidationPreventsRestartAdmission(t *testing.T) {
	m, original := newManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	stop := start
	stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), 2
	if _, err := m.Apply(ctx, stop); err != nil {
		t.Fatal(err)
	}
	b := &overlappingStopBackend{fakeBackend: original, entered: make(chan struct{}), release: make(chan struct{}), validationEntered: make(chan struct{}), validationRelease: make(chan struct{})}
	m.Backend = b
	restart := start
	restart.OperationID, restart.Revision = state.Random(), 3
	started := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, restart); started <- err }()
	select {
	case <-b.validationEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stop.OperationID = state.Random()
	stopped := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, stop); stopped <- err }()
	select {
	case <-b.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second := stop
	second.OperationID = state.Random()
	if _, err := m.Apply(ctx, second); err != nil {
		t.Fatal(err)
	}
	close(b.validationRelease)
	select {
	case err := <-started:
		if !errors.Is(err, ErrPolicy) {
			t.Fatal("validation crossed delayed stop", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(b.release)
	select {
	case err := <-stopped:
		if err != nil && !errors.Is(err, ErrPolicy) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if original.starts != 1 {
		t.Fatal("replacement guest launched", original.starts)
	}
}

func TestRestartWaitsForEveryOverlappingStopEffect(t *testing.T) {
	m, original := newManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	b := &overlappingStopBackend{fakeBackend: original, entered: make(chan struct{}), release: make(chan struct{})}
	m.Backend = b
	released := false
	defer func() {
		if !released {
			close(b.release)
		}
	}()
	stop := start
	stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), 2
	done := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, stop); done <- err }()
	select {
	case <-b.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second := stop
	second.OperationID = state.Random()
	if _, err := m.Apply(ctx, second); err != nil {
		t.Fatal(err)
	}
	restart := start
	restart.OperationID, restart.Revision = state.Random(), 3
	if _, err := m.Apply(ctx, restart); !errors.Is(err, ErrPolicy) {
		t.Fatal("restart crossed unfinished stop", err)
	}
	close(b.release)
	released = true
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, ErrPolicy) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	restart.OperationID = state.Random()
	if _, err := m.Apply(ctx, restart); err != nil {
		t.Fatal("completed stop retained admission", err)
	}
}
