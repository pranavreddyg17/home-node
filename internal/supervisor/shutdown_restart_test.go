package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRestartCannotCrossShutdownAfterEmergencyStop(t *testing.T) {
	m, original := newManager(t)
	ctx := context.Background()
	start := startRequest()
	if _, err := m.Apply(ctx, start); err != nil {
		t.Fatal(err)
	}
	backend := &shutdownFixtureBackend{fakeBackend: original}
	m.Backend = backend
	restart := start
	restart.OperationID, restart.Revision = state.Random(), 4
	backend.during = func() {
		stop := start
		stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), 3
		if _, err := m.Apply(ctx, stop); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Apply(ctx, restart); !errors.Is(err, ErrPolicy) {
			t.Fatal("restart crossed poweroff effect", err)
		}
	}
	shutdown := start
	shutdown.Action, shutdown.OperationID, shutdown.Revision = "shutdown", state.Random(), 2
	if _, err := m.Apply(ctx, shutdown); !errors.Is(err, ErrPolicy) {
		t.Fatal("obsolete shutdown completed", err)
	}
	if original.starts != 1 {
		t.Fatal("guest launched during shutdown", original.starts)
	}
	backend.during = nil
	restart.OperationID = state.Random()
	if _, err := m.Apply(ctx, restart); err != nil {
		t.Fatal("shutdown retained restart admission", err)
	}
}
