package install

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"
)

// Explicitly enabled only in a disposable Linux CI runner after dependency and
// package installation. Ordinary tests never create real host identities.
func TestInstalledAccountInspection(t *testing.T) {
	if os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" {
		t.Skip("disposable supported-host account fixture is opt-in")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("integration requires Linux root")
	}
	ctx := context.Background()
	for _, name := range []string{"homenode", "homenode-transfer"} {
		_, exists, err := lookupAccount(ctx, "passwd", name)
		if err != nil {
			t.Fatal("fixture cannot establish account vacancy", err)
		}
		if exists {
			t.Fatal("fixture refuses existing account", name)
		}
	}
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		_, exists, err := lookupAccount(ctx, "group", name)
		if err != nil {
			t.Fatal("fixture cannot establish group vacancy", err)
		}
		if exists {
			t.Fatal("fixture refuses existing group", name)
		}
	}
	if _, err := accountCommand(ctx, "/usr/bin/getent", "group", "libvirt-qemu"); err != nil {
		t.Fatal("package dependencies must supply the QEMU identity")
	}
	var createdUsers, createdGroups []string
	t.Cleanup(func() {
		for i := len(createdUsers) - 1; i >= 0; i-- {
			_, exists, err := lookupAccount(ctx, "passwd", createdUsers[i])
			if err != nil {
				t.Error("fixture account lookup failed", err)
				continue
			}
			if !exists {
				continue
			}
			if _, err := accountCommand(ctx, "/usr/sbin/userdel", createdUsers[i]); err != nil {
				t.Error("fixture account cleanup failed", createdUsers[i], err)
			}
		}
		for i := len(createdGroups) - 1; i >= 0; i-- {
			_, exists, err := lookupAccount(ctx, "group", createdGroups[i])
			if err != nil {
				t.Error("fixture group lookup failed", createdGroups[i], err)
				continue
			}
			if !exists {
				continue
			}
			if _, err := accountCommand(ctx, "/usr/sbin/groupdel", createdGroups[i]); err != nil {
				t.Error("fixture group cleanup failed", createdGroups[i], err)
			}
		}
	})

	journalDirectory := t.TempDir()
	if err := os.Chmod(journalDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	engine, err := Open("/", journalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	// Initial vacancy was established above. The fixture cleanup runs only for
	// these known names; production account removal remains a separate phase.
	createdGroups = []string{"homenode", "homenode-transfer", "homenode-runtime"}
	createdUsers = []string{"homenode", "homenode-transfer"}
	provisioned, err := engine.ProvisionAccounts(ctx)
	if err != nil {
		t.Fatal("journaled native account creation failed", err)
	}
	replay, err := engine.ProvisionAccounts(ctx)
	if err != nil || replay != provisioned {
		t.Fatal("native provision replay failed", err)
	}

	accounts, err := InspectLocalAccounts(ctx)
	if err != nil || accounts.ControllerUID == accounts.TransferUID {
		t.Fatal("native identity inspection failed", accounts, err)
	}
	output, err := accountCommand(ctx, "/usr/bin/homenode", "accounts-check")
	if err != nil {
		t.Fatal("installed CLI did not inspect accounts", err)
	}
	var result struct {
		Valid     bool     `json:"accountsValid"`
		Accounts  Accounts `json:"accounts"`
		Activated bool     `json:"servicesActivated"`
	}
	if err = json.Unmarshal(output, &result); err != nil || !result.Valid || result.Activated || result.Accounts != accounts {
		t.Fatal("CLI identity result incorrect", err)
	}
	// A real privileged supplementary membership must be denied. The fixture's
	// nologin identity has no password or home, and is removed by cleanup.
	if _, err = accountCommand(ctx, "/usr/sbin/usermod", "--append", "--groups", "root", "homenode"); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectLocalAccounts(ctx); err == nil {
		t.Fatal("privileged supplementary membership accepted")
	}
	if _, err = accountCommand(ctx, "/usr/sbin/usermod", "--groups", "homenode-runtime", "homenode"); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectLocalAccounts(ctx); err != nil {
		t.Fatal("repaired identity not admitted", err)
	}
}
