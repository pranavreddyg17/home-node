package install

import (
	"bytes"
	"context"
	"errors"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
	"os"
	"testing"
)

func TestRecoveryActivationMarkerSurvivesRetryAndRefusesForeignBytes(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.blockRecoveryActivation(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.journalRoot.Lstat("recovery-blocked"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancel created marker", err)
	}
	if err := e.blockRecoveryActivation(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.blockRecoveryActivation(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("marker replaced", err)
	}
	if err = e.requireRecoveryActivationBlock(context.Background()); err != nil {
		t.Fatal("existing activation block refused", err)
	}
	if err = e.journalRoot.Remove("recovery-blocked"); err != nil {
		t.Fatal(err)
	}
	if err = e.requireRecoveryActivationBlock(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing block accepted", err)
	}
	if _, err = e.journalRoot.Lstat("recovery-blocked"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("validation recreated block", err)
	}
	if err = e.blockRecoveryActivation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = e.journalRoot.WriteFile("recovery-blocked", []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.blockRecoveryActivation(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign marker adopted", err)
	}
}

func TestInstalledRecoveryActivationConditionsCoverServicesAndSocket(t *testing.T) {
	for _, name := range []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket"} {
		unit, err := servicetemplates.Unit(name)
		if err != nil || bytes.Count(unit, []byte("ConditionPathExists=!/var/lib/homenode-install/recovery-blocked\n")) != 1 {
			t.Fatal("missing or ambiguous recovery activation condition", name, err)
		}
	}
}
