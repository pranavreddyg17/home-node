package install

import (
	"context"
	"encoding/json"
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
	if !parent.Directory || parent.UID != 0 || parent.GID != 0 || parent.Mode != 0755 || !staging.Directory || staging.UID != 803 || staging.GID != 803 || staging.Mode != 0700 {
		t.Fatal("backup staging ownership is not isolated", parent, staging)
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
	valid := record{Path: "var/lib/homenode-backup/staging", Directory: true, Mode: 0700, UID: 803, GID: 803}
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
