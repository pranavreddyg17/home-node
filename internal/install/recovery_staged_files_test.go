package install

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/backup"
	"testing"
)

func TestStagedRecoveryHandoffRefusesBeforeCallback(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	called := false
	use := func(context.Context, []backup.PreparedRecoveryFile) error { called = true; return nil }
	if err := e.withRecoveryStagedFiles(context.Background(), recoveryIntent{}, use); !errors.Is(err, ErrConflict) || called {
		t.Fatal("unprepared handoff admitted", err)
	}
	if err := e.withRecoveryStagedFiles(context.Background(), recoveryIntent{}, nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing consumer admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.withRecoveryStagedFiles(ctx, recoveryIntent{}, use); !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled handoff admitted", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.withRecoveryStagedFiles(context.Background(), recoveryIntent{}, use); !errors.Is(err, ErrConflict) || called {
		t.Fatal("overlapping handoff admitted", err)
	}
}
