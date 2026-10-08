package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
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

	journalDirectory := installedAccountFixtureJournal(t)
	engine, err := Open("/", journalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.requireRecoveryJournalLocation(ctx); err != nil {
		engine.Close()
		t.Fatal("fixture journal does not bind installed activation path", err)
	}
	defer func() {
		if engine != nil {
			if err := engine.Close(); err != nil {
				t.Error("fixture engine close", err)
			}
		}
	}()
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
	engine.mu.Lock()
	identityOwner, identityErr := engine.inspectGuestIdentityAccountsLocked(ctx)
	engine.mu.Unlock()
	baseIdentity, baseErr := engine.loadAccountJournal()
	if identityErr != nil || baseErr != nil || identityOwner != baseIdentity.OwnerID {
		t.Fatal("identity configuration ownership qualification failed", identityErr, baseErr)
	}
	identityPreview, identityErr := engine.PrepareGuestIdentityConfiguration(ctx)
	if identityErr != nil || identityPreview.OwnerID != identityOwner || !identityPreview.IntentCommitted || identityPreview.ConfigurationApplied || len(identityPreview.OriginalSHA256) != 64 || len(identityPreview.DesiredSHA256) != 64 {
		t.Fatal("native identity preparation failed", identityErr, identityPreview)
	}
	identityReplay, identityErr := engine.PrepareGuestIdentityConfiguration(ctx)
	if identityErr != nil || identityReplay != identityPreview {
		t.Fatal("native identity preparation retry changed intent", identityErr)
	}
	allocatorBefore, err := os.Lstat("/etc/login.defs")
	if err != nil {
		t.Fatal(err)
	}
	allocatorBytes, err := os.ReadFile("/etc/login.defs")
	if err != nil {
		t.Fatal(err)
	}
	allocatorPool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
	allocatorSelection := GuestUIDAllocatorRanges{NormalFirst: 1000, NormalLast: 60000, SystemFirst: 100, SystemLast: 999, SubordinateFirst: 100000, SubordinateLast: 600100000}
	allocatorPreview, err := engine.PrepareGuestUIDAllocationConfiguration(ctx, allocatorPool, allocatorSelection)
	if err != nil || allocatorPreview.OwnerID != identityOwner || allocatorPreview.OriginalSHA256 != digest(allocatorBytes) || len(allocatorPreview.DesiredSHA256) != 64 || !allocatorPreview.IntentCommitted || allocatorPreview.ConfigurationApplied || allocatorPreview.Selection != allocatorSelection {
		t.Fatal("native allocator preparation", err)
	}
	allocatorReplay, err := engine.PrepareGuestUIDAllocationConfiguration(ctx, allocatorPool, allocatorSelection)
	if err != nil || allocatorReplay != allocatorPreview {
		t.Fatal("native allocator preparation retry", err)
	}
	refusedAllocator, allocatorErr := engine.ApplyGuestUIDAllocationConfiguration(ctx)
	if !errors.Is(allocatorErr, ErrConflict) || refusedAllocator != (GuestUIDAllocationPreview{}) {
		t.Fatal("uninstalled allocator application gained authority", allocatorErr)
	}
	allocatorAfter, err := os.Lstat("/etc/login.defs")
	allocatorCurrent, readErr := os.ReadFile("/etc/login.defs")
	if err != nil || readErr != nil || !os.SameFile(allocatorBefore, allocatorAfter) || digest(allocatorCurrent) != digest(allocatorBytes) {
		t.Fatal("allocator preparation changed host configuration", err, readErr)
	}
	// This fixture owns accounts, but deliberately has no installed configuration
	// journal or retained activation barrier. Refusal must precede host mutation.
	identityBefore, identityErr := os.Lstat("/etc/nsswitch.conf")
	if identityErr != nil {
		t.Fatal(identityErr)
	}
	identityBytes, identityErr := os.ReadFile("/etc/nsswitch.conf")
	if identityErr != nil {
		t.Fatal(identityErr)
	}
	refusedPreview, identityErr := engine.ApplyGuestIdentityConfiguration(ctx)
	if !errors.Is(identityErr, ErrConflict) || refusedPreview != (GuestIdentityConfigurationPreview{}) {
		t.Fatal("uninstalled identity application gained authority", identityErr)
	}
	assertIdentityPreserved := func() {
		t.Helper()
		after, err := os.Lstat("/etc/nsswitch.conf")
		current, readErr := os.ReadFile("/etc/nsswitch.conf")
		if err != nil || readErr != nil || !os.SameFile(identityBefore, after) || string(current) != string(identityBytes) {
			t.Fatal("refused identity application changed host configuration", err, readErr)
		}
		if _, err := os.Lstat(filepath.Join(journalDirectory, "guest-identity-nss-stage.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("refused identity application committed a staging record", err)
		}
	}
	assertIdentityPreserved()
	identityEngine := engine
	engine = nil
	if err := identityEngine.Close(); err != nil {
		t.Fatal("identity CLI fixture lock release", err)
	}
	identityOutput, identityErr := accountCommand(ctx, "/usr/bin/homenode", "guest-identity-prepare", "--journal-dir", journalDirectory)
	if identityErr != nil {
		t.Fatal("packaged identity preparation command", identityErr)
	}
	var identityCLI GuestIdentityConfigurationPreview
	if err := json.Unmarshal(identityOutput, &identityCLI); err != nil || identityCLI != identityPreview {
		t.Fatal("packaged identity preparation status", err)
	}
	allocatorOutput, err := accountCommand(ctx, "/usr/bin/homenode", "guest-allocation-prepare", "--first-uid", "2000000000", "--last-uid", "2000000001", "--uid-min", "1000", "--uid-max", "60000", "--sys-uid-min", "100", "--sys-uid-max", "999", "--sub-uid-min", "100000", "--sub-uid-max", "600100000")
	if err != nil {
		t.Fatal("packaged allocator preparation command", err)
	}
	var allocatorCLI GuestUIDAllocationPreview
	if err := json.Unmarshal(allocatorOutput, &allocatorCLI); err != nil || allocatorCLI != allocatorPreview {
		t.Fatal("packaged allocator preparation status", err)
	}
	if _, err := accountCommand(ctx, "/usr/bin/homenode", "guest-identity-apply", "--journal-dir", journalDirectory); err == nil {
		t.Fatal("packaged apply command admitted uninstalled configuration")
	}
	if _, err := accountCommand(ctx, "/usr/bin/homenode", "guest-allocation-apply"); err == nil {
		t.Fatal("packaged allocator apply admitted uninstalled configuration")
	}
	allocatorAfter, err = os.Lstat("/etc/login.defs")
	allocatorCurrent, readErr = os.ReadFile("/etc/login.defs")
	if err != nil || readErr != nil || !os.SameFile(allocatorBefore, allocatorAfter) || digest(allocatorCurrent) != digest(allocatorBytes) {
		t.Fatal("refused packaged allocator application changed host configuration", err, readErr)
	}
	assertIdentityPreserved()
	engine, err = Open("/", journalDirectory)
	if err != nil {
		t.Fatal("identity CLI journal reacquisition", err)
	}
	installAccountFixtureConfiguration(t, engine, accounts, maintenance)
	applyAccountFixtureIdentity(t, &engine, identityPreview)
	applyAccountFixtureAllocator(t, &engine, allocatorPreview)
	storagePrepared := false
	storageQualified := t.Run("QualifiedGuestStorageIntent", func(t *testing.T) {
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
		// Release the real installer flock before invoking its packaged CLI.
		currentEngine := engine
		engine = nil
		if err := currentEngine.Close(); err != nil {
			t.Fatal("fixture lock release", err)
		}
		for _, mode := range []string{"plan", "prepare", "check"} {
			args := []string{"guest-storage-" + mode, "--journal-dir", journalDirectory}
			if mode != "check" {
				args = append(args, "--first-uid", strconv.FormatUint(uint64(pool.First), 10), "--last-uid", strconv.FormatUint(uint64(pool.Last), 10))
			}
			output, err := accountCommand(ctx, "/usr/bin/homenode", args...)
			if err != nil {
				t.Fatal("packaged storage command", mode, err)
			}
			var response struct {
				Plan                GuestStorageProvisioningPlan `json:"plan"`
				IntentCommitted     bool                         `json:"intentCommitted"`
				IntentValid         bool                         `json:"intentValid"`
				PolicyPublished     *bool                        `json:"policyPublished"`
				ServicesActivated   *bool                        `json:"servicesActivated"`
				ActivationQualified *bool                        `json:"activationQualified"`
			}
			if err := json.Unmarshal(output, &response); err != nil || !reflect.DeepEqual(response.Plan, plan) || response.IntentCommitted != (mode == "prepare") || response.IntentValid != (mode == "check") || response.PolicyPublished == nil || *response.PolicyPublished || response.ServicesActivated == nil || *response.ServicesActivated || response.ActivationQualified == nil || *response.ActivationQualified {
				t.Fatal("packaged storage command status", mode, err)
			}
		}
		engine, err = Open("/", journalDirectory)
		if err != nil {
			t.Fatal("fixture journal reacquisition", err)
		}
		storagePrepared = true
	})
	if !storageQualified {
		t.Fatal("native storage qualification failed")
	}
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
