package install

import (
	"strings"
	"testing"
)

func TestConfigurationRemovalManagerAdmitsOnlyOwnedDormantTransition(t *testing.T) {
	unit := "homenode-gateway.service"
	loaded := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
	removed := strings.Replace(loaded, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1)
	absent := strings.Replace(strings.Replace(loaded, "LoadState=loaded", "LoadState=not-found", 1), "FragmentPath=/etc/systemd/system/"+unit, "FragmentPath=", 1)
	for _, observation := range []string{loaded, removed, absent} {
		if err := validateRemovalDormantUnit([]byte(observation), unit, true); err != nil {
			t.Fatal(observation, err)
		}
	}
	for _, observation := range []string{removed, absent} {
		if err := validateRemovalDormantUnit([]byte(observation), unit, false); err == nil {
			t.Fatal("unrecorded unit disappearance admitted")
		}
	}
	for _, bad := range []string{
		strings.Replace(removed, "ActiveState=inactive", "ActiveState=active", 1),
		strings.Replace(removed, "MainPID=0", "MainPID=12", 1),
		strings.Replace(removed, "Job=", "Job=23", 1),
		strings.Replace(removed, "DropInPaths=", "DropInPaths=/etc/systemd/system/foreign.conf", 1),
		strings.Replace(removed, "FragmentPath=/etc/systemd/system/", "FragmentPath=/usr/lib/systemd/system/", 1),
		strings.Replace(absent, "LoadState=not-found", "LoadState=masked", 1),
		strings.Replace(absent, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1),
		absent + "Id=" + unit + "\n",
	} {
		if err := validateRemovalDormantUnit([]byte(bad), unit, true); err == nil {
			t.Fatal("unsafe transition admitted", bad)
		}
	}
	if err := validateRemovalDormantUnit([]byte(absent), "foreign.service", true); err == nil {
		t.Fatal("foreign unit admitted")
	}
	// Teardown admission must not relax normal recovery admission.
	if err := validateRecoveryDormantUnit([]byte(removed), unit); err == nil {
		t.Fatal("recovery admitted pending reload")
	}
}

func TestConfigurationRemovalManagerCoversInspectionSocketsAndSlice(t *testing.T) {
	for _, unit := range []string{"homenode-app-maintenance.socket", "homenode-backup-credential.socket", "homenode-inspect.service", "homenode.slice"} {
		observation := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\nActiveState=inactive\nSubState=dead\n"
		if strings.HasSuffix(unit, ".service") {
			observation += "MainPID=0\nControlPID=0\n"
		}
		if err := validateRemovalDormantUnit([]byte(observation), unit, false); err != nil {
			t.Fatal(unit, err)
		}
		if err := validateRemovalDormantUnit([]byte(strings.Replace(observation, "ActiveState=inactive", "ActiveState=active", 1)), unit, false); err == nil {
			t.Fatal("active uninstall dependency admitted", unit)
		}
	}
}

func TestConfigurationRemovalUnitsOrderAndExactAllowlist(t *testing.T) {
	plan := ConfigurationRemovalPlan{Files: []ConfigurationRemovalFile{
		{Path: "etc/systemd/system/homenode.slice"},
		{Path: "etc/homenode/services.env"},
		{Path: "etc/systemd/system/homenode-inspect.service"},
		{Path: "etc/systemd/system/homenode-app-maintenance.socket"},
		{Path: "etc/systemd/system/homenode-gateway.service"},
	}}
	units, err := configurationRemovalUnits(plan)
	if err != nil || strings.Join(units, ",") != "homenode-app-maintenance.socket,homenode-gateway.service,homenode-inspect.service,homenode.slice" {
		t.Fatal(units, err)
	}
	plan.Files = append(plan.Files, ConfigurationRemovalFile{Path: "etc/systemd/system/foreign.service"})
	if _, err := configurationRemovalUnits(plan); err == nil {
		t.Fatal("foreign manager operation selected")
	}
}

func TestConfigurationRemovalLoadedOwnershipDoesNotAdmitAbsentOrForeignUnits(t *testing.T) {
	unit := "homenode-inspect.service"
	observation := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\n"
	if err := validateRemovalLoadedUnit([]byte(observation), unit); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(observation, "LoadState=loaded", "LoadState=not-found", 1), strings.Replace(observation, "Job=", "Job=12", 1), strings.Replace(observation, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1), observation + "ActiveState=active\n"} {
		if err := validateRemovalLoadedUnit([]byte(bad), unit); err == nil {
			t.Fatal("unknown or changed loaded unit admitted", bad)
		}
	}
	if err := validateRemovalLoadedUnit([]byte(observation), "foreign.service"); err == nil {
		t.Fatal("foreign stop target admitted")
	}
}

func TestConfigurationRemovalRetirementRequiresAbsentDormantJobFreeManagerState(t *testing.T) {
	for _, unit := range []string{"homenode-control.service", "homenode-app-maintenance.socket", "homenode-inspect.service", "homenode.slice"} {
		absent := "Id=" + unit + "\nFragmentPath=\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=not-found\nActiveState=inactive\nSubState=dead\n"
		if strings.HasSuffix(unit, ".service") {
			absent += "MainPID=0\nControlPID=0\n"
		}
		if err := validateRemovalRetiredUnit([]byte(absent), unit); err != nil {
			t.Fatal(unit, err)
		}
		for _, bad := range []string{
			strings.Replace(absent, "LoadState=not-found", "LoadState=loaded", 1),
			strings.Replace(absent, "Job=", "Job=123", 1),
			strings.Replace(absent, "ActiveState=inactive", "ActiveState=active", 1),
			strings.Replace(absent, "FragmentPath=", "FragmentPath=/usr/lib/systemd/system/"+unit, 1),
			strings.Replace(absent, "LoadState=not-found", "LoadState=masked", 1),
		} {
			if err := validateRemovalRetiredUnit([]byte(bad), unit); err == nil {
				t.Fatal("unretired manager state admitted", unit, bad)
			}
		}
	}
}
