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
	observerFailure := errors.New("manager observation unavailable")
	markerBefore, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), func(context.Context) error { return observerFailure }); !errors.Is(err, observerFailure) {
		t.Fatal("manager failure lost", err)
	}
	markerAfter, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(markerBefore, markerAfter) {
		t.Fatal("manager failure changed activation block", err)
	}
	policyPath := "etc/homenode/runtime-policy.json"
	originalPolicy, err := e.host.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	changePolicy := func(context.Context) error {
		return e.host.WriteFile(policyPath, []byte("changed during observation"), 0600)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), changePolicy); !errors.Is(err, ErrConflict) {
		t.Fatal("configuration drift during observation accepted", err)
	}
	if err = e.host.WriteFile(policyPath, originalPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	unitPath := "etc/systemd/system/homenode-control.service"
	originalUnit, err := e.host.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	changeUnit := func(context.Context) error {
		return e.host.WriteFile(unitPath, []byte("[Unit]\nDescription=foreign\n[Service]\nExecStart=/bin/true\n"), 0644)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), changeUnit); !errors.Is(err, ErrConflict) {
		t.Fatal("unit drift during observation accepted", err)
	}
	if err = e.host.WriteFile(unitPath, originalUnit, 0644); err != nil {
		t.Fatal(err)
	}
	originalJournal, err := e.journalRoot.ReadFile("install.json")
	if err != nil {
		t.Fatal(err)
	}
	changeJournal := func(context.Context) error {
		current, err := e.load()
		if err != nil {
			return err
		}
		current.ID = strings.Repeat("b", 32)
		return e.save(current)
	}
	if err = e.observeRecoveryQuiescence(context.Background(), changeJournal); !errors.Is(err, ErrConflict) {
		t.Fatal("installation identity changed during observation accepted", err)
	}
	if err = e.journalRoot.WriteFile("install.json", originalJournal, 0600); err != nil {
		t.Fatal(err)
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
