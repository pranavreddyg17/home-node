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

func TestConfigurationRemovalExclusionRunsJournaledConsumerAndRetainsData(t *testing.T) {
	e, host, _ := configurationRemoveFixture(t)
	defer e.Close()
	ctx := context.Background()
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	dataPath := host + "/var/lib/homenode/control/management.db"
	if err := os.WriteFile(dataPath, []byte("owner data"), 0600); err != nil {
		t.Fatal(err)
	}
	observe := func(context.Context) error { return nil } // Simulated manager only.
	e.mu.Lock()
	err = e.removeConfigurationWithExclusionLocked(ctx, observe)
	e.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	err = e.removeConfigurationWithExclusionLocked(ctx, observe)
	e.mu.Unlock()
	if err != nil {
		t.Fatal("ready retry", err)
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("consumer released activation", err)
	}
	data, err := os.ReadFile(dataPath)
	if err != nil || string(data) != "owner data" {
		t.Fatal("consumer deleted owner data", err)
	}
}

func TestConfigurationRemovalExclusionLosingMarkerBeforeAcknowledgementStopsNextUnlink(t *testing.T) {
	e, _, _ := configurationRemoveFixture(t)
	defer e.Close()
	ctx := context.Background()
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	e.checkpoint = func(stage, name string) error {
		if stage == "configuration-removed" {
			if err := e.journalRoot.Remove("recovery-blocked"); err != nil {
				return err
			}
			return e.blockRecoveryActivation(ctx)
		}
		return nil
	}
	e.mu.Lock()
	err := e.removeConfigurationWithExclusionLocked(ctx, func(context.Context) error { return nil })
	e.mu.Unlock()
	if !errors.Is(err, ErrConflict) {
		t.Fatal("lost exclusion acknowledged", err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil || intent.Completed != 0 || intent.Ready {
		t.Fatal("loss committed acknowledgement", intent, err)
	}
	if _, err := e.host.Lstat("etc/homenode/services.env"); err != nil {
		t.Fatal("loss allowed next unlink", err)
	}
}
