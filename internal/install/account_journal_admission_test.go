package install

import (
	"bytes"
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

func TestOwnershipJournalsRefuseSymlinksAndOversizedRecords(t *testing.T) {
	for _, name := range []string{"accounts.json", "maintenance-accounts.json"} {
		for _, mutation := range []string{"symlink", "oversized"} {
			t.Run(name+"/"+mutation, func(t *testing.T) {
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
				path := filepath.Join(jr, name)
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				preservedPath := path
				if mutation == "symlink" {
					preservedPath = filepath.Join(jr, "preserved-intent")
					if err := os.Rename(path, preservedPath); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink("preserved-intent", path); err != nil {
						t.Fatal(err)
					}
				} else {
					// A valid JSON record followed by whitespace must not hide an oversized
					// file behind the decoder's bounded reader.
					original = append(original, bytes.Repeat([]byte(" "), (64<<10)+1)...)
					if err := os.WriteFile(path, original, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if name == "accounts.json" {
					if _, err := engine.loadAccountJournal(); err == nil {
						t.Fatal("unsafe base intent admitted")
					}
					if _, err := engine.provisionAccounts(context.Background(), backend); err == nil {
						t.Fatal("unsafe base intent authorized commands")
					}
				} else {
					if _, err := engine.loadMaintenanceAccountJournal(base); err == nil {
						t.Fatal("unsafe maintenance intent admitted")
					}
					if _, err := engine.prepareMaintenanceAccount(context.Background(), backend); err == nil {
						t.Fatal("unsafe maintenance intent replayed")
					}
				}
				after, err := os.ReadFile(preservedPath)
				if err != nil || !bytes.Equal(original, after) || backend.commands != 5 {
					t.Fatal("refusal modified intent or repeated commands", err, backend.commands)
				}
			})
		}
	}
}
