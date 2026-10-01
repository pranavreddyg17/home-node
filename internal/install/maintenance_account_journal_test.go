package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMaintenanceAccountIntentPersistsWithoutMutation(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	backend := newAccountFixture()
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	plan, err := engine.prepareMaintenanceAccount(context.Background(), backend)
	if err != nil || backend.commands != 5 {
		t.Fatal(plan, backend.commands, err)
	}
	info, err := os.Stat(filepath.Join(journal, "maintenance-accounts.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	if err = engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine = openEngine(t, host, journal)
	defer engine.Close()
	replay, err := engine.prepareMaintenanceAccount(context.Background(), backend)
	if err != nil || !reflect.DeepEqual(replay, plan) || backend.commands != 5 {
		t.Fatal("intent replay changed", replay, err)
	}
	data, err := os.ReadFile(filepath.Join(journal, "maintenance-accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(journal, "maintenance-accounts.json"), append(data, []byte("\n{}")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.prepareMaintenanceAccount(context.Background(), backend); !errors.Is(err, ErrConflict) {
		t.Fatal("altered intent accepted", err)
	}
}

func TestMaintenanceAccountIntentRefusesUnownedPhaseAndNSSCollision(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	backend := newAccountFixture()
	if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); !errors.Is(err, ErrConflict) {
		t.Fatal("unowned base accepted", err)
	}
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	backend.collide = true
	if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); !errors.Is(err, ErrConflict) {
		t.Fatal("NSS collision accepted", err)
	}
	if _, err := os.Lstat(filepath.Join(journal, "maintenance-accounts.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("colliding intent committed", err)
	}
	backend.collide = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.prepareMaintenanceAccount(ctx, backend); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled intent committed", err)
	}
	if _, err := os.Lstat(filepath.Join(journal, "maintenance-accounts.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	backend.s.passwd = []byte(strings.Replace(string(backend.s.passwd), "HomeNode install ", "Foreign install ", 1))
	if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); !errors.Is(err, ErrConflict) {
		t.Fatal("lost base ownership accepted", err)
	}

}
