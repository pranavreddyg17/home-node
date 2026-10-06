package install

import (
	"context"
	"errors"
	"os"
	"strings"
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

func TestRootRecoveryQuiescenceRejectsMarkerReplacementDuringObservation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	c, _, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	c.BackupRepositoryID = strings.Repeat("a", 64)
	c.BackupRelease = "0.1.0"
	c.BackupDriveUUID = "abcd-1234"
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	if err = e.Apply(context.Background(), preview.Plan); err != nil {
		t.Fatal(err)
	}
	if err = e.blockRecoveryActivation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal("owned preflight", err)
	}
	replace := func(ctx context.Context) error {
		if err := e.journalRoot.Remove("recovery-blocked"); err != nil {
			return err
		}
		return e.blockRecoveryActivation(ctx)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), replace); !errors.Is(err, ErrConflict) {
		t.Fatal("replaced activation marker accepted", err)
	}
}
