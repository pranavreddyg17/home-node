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
		if _, err := accountCommand(ctx, "/usr/bin/getent", "passwd", name); err == nil {
			t.Fatal("fixture refuses existing account", name)
		}
	}
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		if _, err := accountCommand(ctx, "/usr/bin/getent", "group", name); err == nil {
			t.Fatal("fixture refuses existing group", name)
		}
	}
	if _, err := accountCommand(ctx, "/usr/bin/getent", "group", "libvirt-qemu"); err != nil {
		t.Fatal("package dependencies must supply the QEMU identity")
	}
	var createdUsers, createdGroups []string
	t.Cleanup(func() {
		for i := len(createdUsers) - 1; i >= 0; i-- {
			if _, err := accountCommand(ctx, "/usr/sbin/userdel", createdUsers[i]); err != nil {
				t.Error("fixture account cleanup failed", createdUsers[i], err)
			}
		}
		for i := len(createdGroups) - 1; i >= 0; i-- {
			if _, err := accountCommand(ctx, "/usr/sbin/groupdel", createdGroups[i]); err != nil {
				t.Error("fixture group cleanup failed", createdGroups[i], err)
			}
		}
	})
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		if _, err := accountCommand(ctx, "/usr/sbin/groupadd", "--system", name); err != nil {
			t.Fatal(err)
		}
		createdGroups = append(createdGroups, name)
	}
	for _, name := range []string{"homenode", "homenode-transfer"} {
		if _, err := accountCommand(ctx, "/usr/sbin/useradd", "--system", "--gid", name, "--groups", "homenode-runtime", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", name); err != nil {
			t.Fatal(err)
		}
		createdUsers = append(createdUsers, name)
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
