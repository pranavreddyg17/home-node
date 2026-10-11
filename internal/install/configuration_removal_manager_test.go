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
