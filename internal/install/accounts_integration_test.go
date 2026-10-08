package install

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
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
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-backup"} {
		_, exists, err := lookupAccount(ctx, "passwd", name)
		if err != nil {
			t.Fatal("fixture cannot establish account vacancy", err)
		}
		if exists {
			t.Fatal("fixture refuses existing account", name)
		}
	}
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime", "homenode-backup"} {
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
	createdGroups = []string{"homenode", "homenode-transfer", "homenode-runtime", "homenode-backup"}
	createdUsers = []string{"homenode", "homenode-transfer", "homenode-backup"}
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
	plan, err := engine.PrepareMaintenanceAccount(ctx)
	if err != nil {
		t.Fatal("native maintenance preparation", err)
	}
	maintenance, err := engine.ProvisionMaintenanceAccount(ctx)
	if err != nil || maintenance != plan.Identity {
		t.Fatal("native maintenance provisioning", maintenance, err)
	}
	if replay, err := engine.ProvisionMaintenanceAccount(ctx); err != nil || replay != maintenance {
		t.Fatal("native maintenance replay", replay, err)
	}
	output, err = accountCommand(ctx, "/usr/bin/homenode", "maintenance-accounts-check")
	if err != nil {
		t.Fatal("installed CLI backup inspection", err)
	}
	var backupResult struct {
		Valid     bool               `json:"maintenanceAccountValid"`
		Identity  MaintenanceAccount `json:"identity"`
		Activated bool               `json:"servicesActivated"`
	}
	if err = json.Unmarshal(output, &backupResult); err != nil || !backupResult.Valid || backupResult.Activated || backupResult.Identity != maintenance {
		t.Fatal("CLI backup identity result", backupResult, err)
	}
	storagePrepared := false
	t.Run("QualifiedGuestStorageIntent", func(t *testing.T) {
		device, err := os.Lstat("/dev/kvm")
		if os.IsNotExist(err) {
			t.Skip("native installed storage intent unverified: KVM is absent")
		}
		if err != nil || device.Mode()&os.ModeCharDevice == 0 {
			t.Fatal("ambiguous KVM fixture", err)
		}
		pool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
		plan, err := engine.PrepareGuestStorageProvisioning(ctx, pool)
		if err != nil {
			t.Fatal("native installed storage preparation", err)
		}
		if plan.GuestGID == 0 || len(plan.Identity.ServiceUIDs) != 3 || plan.ParentMode != 0710 || plan.ImageMode != 0440 || plan.VolumeMode != 0600 {
			t.Fatal("unqualified installed storage proposal", plan)
		}
		replayed, err := engine.PrepareGuestStorageProvisioning(ctx, pool)
		if err != nil || !reflect.DeepEqual(replayed, plan) {
			t.Fatal("native storage exact retry", replayed, err)
		}
		checked, err := engine.CheckGuestStorageProvisioningIntent(ctx)
		if err != nil || !reflect.DeepEqual(checked, plan) {
			t.Fatal("native storage revalidation", checked, err)
		}
		storagePrepared = true
	})
	if _, err = accountCommand(ctx, "/usr/sbin/usermod", "--append", "--groups", "root", "homenode-backup"); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectMaintenanceAccount(ctx); err == nil {
		t.Fatal("privileged backup supplementary membership accepted")
	}
	if _, err = engine.ProvisionMaintenanceAccount(ctx); err == nil {
		t.Fatal("unsafe backup readiness replay accepted")
	}
	if storagePrepared {
		if _, err := engine.CheckGuestStorageProvisioningIntent(ctx); err == nil {
			t.Fatal("storage intent admitted privileged maintenance account drift")
		}
	}
	if _, err = accountCommand(ctx, "/usr/sbin/usermod", "--groups", "", "homenode-backup"); err != nil {
		t.Fatal(err)
	}
	if inspected, err := InspectMaintenanceAccount(ctx); err != nil || inspected != maintenance {
		t.Fatal("repaired backup identity", inspected, err)
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
