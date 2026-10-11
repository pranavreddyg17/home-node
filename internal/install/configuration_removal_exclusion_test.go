package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestConfigurationRemovalExclusionRetainsMarkerAcrossInterruptedDeletion(t *testing.T) {
	e, _, _ := configurationRemoveFixture(t)
	defer e.Close()
	ctx := context.Background()
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	stopped := errors.New("interrupted consumer")
	called := false
	e.mu.Lock()
	err = e.withConfigurationRemovalExclusionLocked(ctx, func(context.Context) error { return nil }, func(ctx context.Context, guard func(context.Context) error) error {
		called = true
		if err := guard(ctx); err != nil {
			return err
		}
		return stopped
	})
	e.mu.Unlock()
	if !errors.Is(err, stopped) || !called {
		t.Fatal(err, called)
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("interruption released marker", err)
	}
}

func TestConfigurationRemovalExclusionRejectsReplacedMarkerBeforeConsumer(t *testing.T) {
	e, _, _ := configurationRemoveFixture(t)
	defer e.Close()
	ctx := context.Background()
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	called := false
	observe := func(ctx context.Context) error {
		if err := e.journalRoot.Remove("recovery-blocked"); err != nil {
			return err
		}
		return e.blockRecoveryActivation(ctx)
	}
	e.mu.Lock()
	err := e.withConfigurationRemovalExclusionLocked(ctx, observe, func(context.Context, func(context.Context) error) error { called = true; return nil })
	e.mu.Unlock()
	if !errors.Is(err, ErrConflict) || called {
		t.Fatal("replacement granted consumer access", err, called)
	}
}
