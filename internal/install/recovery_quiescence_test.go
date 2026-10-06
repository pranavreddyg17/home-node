package install

import (
	"context"
	"errors"
	"testing"
)

func TestRecoveryQuiescenceRefusesMissingInstalledActivationConditions(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	called := false
	observe := func(context.Context) error { called = true; return nil }
	if err := e.observeRecoveryQuiescence(context.Background(), observe); !errors.Is(err, ErrConflict) || called {
		t.Fatal("uninstalled host queried", err)
	}
	if err := e.Apply(context.Background(), fixturePlan()); err != nil {
		t.Fatal(err)
	}
	if err := e.blockRecoveryActivation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.observeRecoveryQuiescence(context.Background(), observe); !errors.Is(err, ErrConflict) || called {
		t.Fatal("unguarded installation queried", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.observeRecoveryQuiescence(ctx, observe); !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled preflight queried", err)
	}
}
