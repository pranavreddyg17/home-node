package install

import (
	"context"
	"os"
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
