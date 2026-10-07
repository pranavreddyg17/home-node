package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountOwnershipJournalRefusesAliasAndPreservesBytes(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	backend := newAccountFixture()
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(jr, "accounts.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(jr, "foreign-alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadAccountJournal(); err == nil {
		t.Fatal("hard-linked ownership journal admitted")
	}
	if _, err := engine.provisionAccounts(context.Background(), backend); err == nil {
		t.Fatal("hard-linked intent authorized account provision")
	}
	preserved, err := os.ReadFile(alias)
	if err != nil || string(preserved) != string(original) || backend.commands != 5 {
		t.Fatal("foreign alias changed or commands repeated", err, backend.commands)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadAccountJournal(); err != nil {
		t.Fatal("unaliased journal no longer admitted", err)
	}
	if err := os.Chmod(path, 0600|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadAccountJournal(); err == nil {
		t.Fatal("special-bit ownership journal admitted")
	}
}

func TestMaintenanceOwnershipJournalRefusesAlias(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	backend := newAccountFixture()
	if _, err := engine.provisionAccounts(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); err != nil {
		t.Fatal(err)
	}
	base, err := engine.loadAccountJournal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(jr, "maintenance-accounts.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(jr, "backup-foreign-alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadMaintenanceAccountJournal(base); err == nil {
		t.Fatal("aliased maintenance intent admitted")
	}
	if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); err == nil {
		t.Fatal("aliased maintenance intent replayed")
	}
	preserved, err := os.ReadFile(alias)
	if err != nil || string(preserved) != string(original) || backend.commands != 5 {
		t.Fatal("maintenance alias changed", err, backend.commands)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadMaintenanceAccountJournal(base); err != nil {
		t.Fatal("single-link intent refused", err)
	}
}
