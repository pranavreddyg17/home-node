package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceConfigurationUsesObservedDistinctIdentity(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	parent := findConfiguration(t, preview.Plan, "var/lib/homenode-backup")
	staging := findConfiguration(t, preview.Plan, "var/lib/homenode-backup/staging")
	recovery := findConfiguration(t, preview.Plan, "var/lib/homenode-backup/recovery")
	if !parent.Directory || parent.UID != 0 || parent.GID != 0 || parent.Mode != 0755 || !staging.Directory || staging.UID != 803 || staging.GID != 803 || staging.Mode != 0700 {
		t.Fatal("backup staging ownership is not isolated", parent, staging)
	}
	if !recovery.Directory || recovery.Mode != 0700 || recovery.UID != 803 || recovery.GID != 803 {
		t.Fatal("recovery staging ownership is not isolated", recovery)
	}
	unit := string(findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-supervisor.service").Data)
	if !strings.Contains(unit, "--maintenance-uid 803 --maintenance-gid 803\n") {
		t.Fatal("maintenance socket omitted", unit)
	}
	control := string(findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-control.service").Data)
	if !strings.Contains(control, "--maintenance-uid 803 --maintenance-gid 803\n") || !strings.Contains(control, "homenode-transfer.service homenode-app-maintenance.socket\n") {
		t.Fatal("controller activation omitted", control)
	}
	socket := string(findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-app-maintenance.socket").Data)
	for _, required := range []string{"SocketUser=root\n", "SocketGroup=homenode-backup\n", "SocketMode=0660\n", "FileDescriptorName=homenode-app-maintenance\n", "Service=homenode-control.service\n"} {
		if !strings.Contains(socket, required) {
			t.Fatal("unsafe activation socket", socket)
		}
	}
	for _, identity := range []MaintenanceAccount{{UID: 0, GID: 803}, {UID: 800, GID: 803}, {UID: 803, GID: 802}, {UID: 1000, GID: 803}} {
		c.Maintenance = &identity
		if _, err := ConfigurationPlan(c, now); err == nil {
			t.Fatal("unsafe identity admitted", identity)
		}
	}
}

func TestBackupStagingPlanAdmissionIsPrivateAndBounded(t *testing.T) {
	for _, name := range []string{"var/lib/homenode-backup/staging", "var/lib/homenode-backup/recovery"} {
		valid := record{Path: name, Directory: true, Mode: 0700, UID: 803, GID: 803}
		if !validRecord(valid, 0) {
			t.Fatal("private staging refused")
		}
		for _, change := range []func(*record){
			func(r *record) { r.Mode = 0755 },
			func(r *record) { r.Mode = 0770 },
			func(r *record) { r.UID = 0 },
			func(r *record) { r.UID = 1000 },
			func(r *record) { r.GID = 0 },
			func(r *record) { r.GID = 1000 },
			func(r *record) { r.Path += "/child" },
			func(r *record) { r.SHA256 = strings.Repeat("a", 64) },
			func(r *record) { r.Directory = false },
		} {
			candidate := valid
			change(&candidate)
			if validRecord(candidate, 0) {
				t.Fatal("unsafe staging record admitted", candidate)
			}
		}
	}
	c, _, _, now := configurationFixture(t)
	c.Maintenance = nil
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range preview.Plan.Items {
		if strings.HasPrefix(item.Path, "var/lib/homenode-backup") {
			t.Fatal("backup paths provisioned without isolated identity", item)
		}
	}
}

func TestRootMaintenanceConfigurationIgnoresSuppliedIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary configuration fixture")
	}
	c, _, _, now := configurationFixture(t)
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	readyAccountIntent(t, engine, c.Accounts, true)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	// The internal default observer cannot adopt supplied backup IDs.
	preview, err := engine.configure(context.Background(), c, now, func(context.Context) (Accounts, Capacity, error) { return c.Accounts, c.Capacity, nil })
	if err != nil {
		t.Fatal(err)
	}
	unit := string(findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-supervisor.service").Data)
	if preview.Maintenance != nil || strings.Contains(unit, "--maintenance-uid") {
		t.Fatal("caller-selected backup identity adopted")
	}
}

func TestRootMaintenanceConfigurationBindsObservedIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary configuration fixture")
	}
	c, _, _, now := configurationFixture(t)
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	readyAccountIntent(t, engine, c.Accounts, true)
	c.Maintenance = &MaintenanceAccount{UID: 0, GID: 0}
	observed := MaintenanceAccount{UID: 803, GID: 803}
	preview, err := engine.configureWithMaintenance(context.Background(), c, now, func(context.Context) (Accounts, Capacity, error) { return c.Accounts, c.Capacity, nil }, func(context.Context, accountJournal) (*MaintenanceAccount, error) { return &observed, nil })
	if err != nil || preview.Maintenance == nil || *preview.Maintenance != observed {
		t.Fatal("observed identity not bound", preview.Maintenance, err)
	}
	unit := string(findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-supervisor.service").Data)
	if !strings.Contains(unit, "--maintenance-uid 803 --maintenance-gid 803\n") {
		t.Fatal("observed socket identity missing")
	}
}

func TestRootPreparedMaintenanceSocketConfiguration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable root-owned configuration fixture")
	}
	for _, scenario := range []string{"valid", "changed-controller", "changed-socket", "missing-socket", "public-staging", "unfinished-account"} {
		t.Run(scenario, func(t *testing.T) {
			identity := MaintenanceAccount{UID: 803, GID: 803}
			engine, c, host, _, source, now := imagePlacementFixtureWithMaintenance(t, &identity)
			defer engine.Close()
			readyAccountIntent(t, engine, c.Accounts, true)
			base, err := engine.loadAccountJournal()
			if err != nil {
				t.Fatal(err)
			}
			journal := maintenanceAccountJournal{Version: 1, Completed: 2, Ready: true, Plan: MaintenanceAccountPlan{OwnerID: base.OwnerID, Identity: identity, Commands: maintenanceCreationCommands(base.OwnerID, identity)}}
			if scenario == "unfinished-account" {
				journal.Ready = false
			}
			data, err := json.Marshal(journal)
			if err != nil {
				t.Fatal(err)
			}
			if err = engine.saveJournalBytes("maintenance-accounts", data); err != nil {
				t.Fatal(err)
			}
			if err = engine.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "changed-controller":
				err = os.WriteFile(filepath.Join(host, "etc/systemd/system/homenode-control.service"), []byte("[Service]\nExecStart=/bin/true\n"), 0644)
			case "changed-socket":
				err = os.WriteFile(filepath.Join(host, "etc/systemd/system/homenode-app-maintenance.socket"), []byte("[Socket]\nSocketMode=0666\n"), 0644)
			case "missing-socket":
				err = os.Remove(filepath.Join(host, "etc/systemd/system/homenode-app-maintenance.socket"))
			case "public-staging":
				err = os.Chmod(filepath.Join(host, "var/lib/homenode-backup/staging"), 0755)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.checkPrepared(context.Background(), now)
			if scenario == "valid" {
				if err != nil || result.Maintenance == nil || *result.Maintenance != identity {
					t.Fatal("owned socket preparation refused", result, err)
				}
				if result.AccountsVerified {
					t.Fatal("modeled journals claimed live accounts")
				}
			} else if err == nil {
				t.Fatal("unsafe maintenance configuration accepted", scenario)
			}
		})
	}
}

func TestPreparedStagingRequiresJournalBoundIdentity(t *testing.T) {
	identity := &MaintenanceAccount{UID: 803, GID: 803}
	parent := record{Path: "var/lib/homenode-backup", Directory: true, Mode: 0755}
	staging := record{Path: "var/lib/homenode-backup/staging", Directory: true, Mode: 0700, UID: 803, GID: 803}
	recovery := staging
	recovery.Path = "var/lib/homenode-backup/recovery"
	if err := validateMaintenanceStaging(journal{Items: []record{parent, staging, recovery}}, identity); err != nil {
		t.Fatal(err)
	}
	wrongRecovery := recovery
	wrongRecovery.UID = 804
	for _, items := range [][]record{{parent, staging, wrongRecovery}, {parent, staging, recovery, recovery}} {
		if err := validateMaintenanceStaging(journal{Items: items}, identity); err == nil {
			t.Fatal("foreign/duplicated recovery staging adopted", items)
		}
	}
	if err := validateMaintenanceStaging(journal{Items: []record{parent, staging}}, identity); err != nil {
		t.Fatal(err)
	}
	if err := validateMaintenanceStaging(journal{}, nil); err != nil {
		t.Fatal(err)
	}
	wrong := staging
	wrong.UID = 804
	for _, items := range [][]record{nil, {parent}, {staging}, {parent, parent}, {parent, staging, staging}, {parent, wrong}} {
		if err := validateMaintenanceStaging(journal{Items: items}, identity); err == nil {
			t.Fatal("incomplete or foreign staging adopted", items)
		}
	}
	if err := validateMaintenanceStaging(journal{Items: []record{parent, staging}}, nil); err == nil {
		t.Fatal("backup paths adopted without maintenance identity")
	}
}

func TestRegisteredBackupApprovalConfiguration(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.BackupRepositoryID = strings.Repeat("a", 64)
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("repository without isolated maintenance identity accepted")
	}
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	unit := findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-control.service")
	if !strings.Contains(string(unit.Data), "--maintenance-uid 803 --maintenance-gid 803 --backup-repository-id "+c.BackupRepositoryID+"\n") || preview.BackupRepositoryID != c.BackupRepositoryID || unit.UID != 0 || unit.Mode != 0644 {
		t.Fatal("registered approval configuration omitted", string(unit.Data))
	}
	for _, invalid := range []string{"foreign", strings.Repeat("A", 64), strings.Repeat("a", 63), "value\nExecStart=evil"} {
		c.BackupRepositoryID = invalid
		if _, err := ConfigurationPlan(c, now); err == nil {
			t.Fatal("unsafe repository identity accepted")
		}
	}
}

func TestBackupExecutionUnitUsesVerifiedCatalogAndValidatedRelease(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	c.BackupRepositoryID = strings.Repeat("a", 64)
	c.BackupRelease = "0.1.0"
	c.BackupDriveUUID = "abcd-1234"
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	unit := findConfiguration(t, preview.Plan, "etc/systemd/system/homenode-control.service")
	expected := fmt.Sprintf("--backup-release 0.1.0 --backup-catalog-version %d\n", preview.CatalogVersion)
	if !strings.Contains(string(unit.Data), expected) || preview.BackupRelease != c.BackupRelease || unit.UID != 0 || unit.Mode != 0644 {
		t.Fatal("installed execution metadata omitted", string(unit.Data))
	}
	for _, invalid := range []string{"invalid", "0.1.0\nExecStart=evil", "0.1.0 --flag", "0.1.0${VALUE}"} {
		c.BackupRelease = invalid
		if _, err := ConfigurationPlan(c, now); err == nil {
			t.Fatal("unsafe release accepted", invalid)
		}
	}
	c.BackupRelease = "0.1.0"
	c.BackupDriveUUID = "abcd-1234"
	c.BackupRepositoryID = ""
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("execution without repository accepted")
	}
}

func TestBackupWorkerEnvironmentIsFixedAndRootOwned(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	c.BackupRepositoryID = strings.Repeat("a", 64)
	c.BackupRelease = "0.1.0"
	c.BackupDriveUUID = "abcd-1234"
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	env := findConfiguration(t, preview.Plan, "etc/homenode/backup.env")
	expected := fmt.Sprintf("CONTROLLER_UID=%d\nCONTROLLER_GID=%d\nBACKUP_UUID=abcd-1234\nBACKUP_REPOSITORY_ID=%s\nINSTALLED_RELEASE=0.1.0\nCATALOG_VERSION=%d\nMINIMUM_CATALOG_VERSION=%d\n", c.Accounts.ControllerUID, c.Accounts.ControllerGID, c.BackupRepositoryID, preview.CatalogVersion, c.MinimumCatalogVersion)
	if string(env.Data) != expected || env.UID != 0 || env.GID != 0 || env.Mode != 0600 || preview.BackupDriveUUID != c.BackupDriveUUID {
		t.Fatal("worker environment mismatched", string(env.Data))
	}
	for _, uuid := range []string{"", "../sda", "abcd\nSECRET=evil", "abcd${VALUE}", "abcd%specifier"} {
		c.BackupDriveUUID = uuid
		if _, err := ConfigurationPlan(c, now); err == nil {
			t.Fatal("unsafe UUID accepted", uuid)
		}
	}
	c.BackupDriveUUID = "abcd-1234"
	c.BackupRelease = ""
	if _, err := ConfigurationPlan(c, now); err == nil {
		t.Fatal("drive without release accepted")
	}
}
