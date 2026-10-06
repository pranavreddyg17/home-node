package install

import (
	"errors"
	"strings"
	"testing"
)

func TestRecoveryDormancyRequiresExactInactiveProcessState(t *testing.T) {
	valid := "LoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
	if err := validateRecoveryDormant([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(valid, "inactive", "active", 1),
		strings.Replace(valid, "dead", "stop", 1),
		strings.Replace(valid, "MainPID=0", "MainPID=123", 1),
		strings.Replace(valid, "ControlPID=0", "ControlPID=123", 1),
		strings.Replace(valid, "loaded", "not-found", 1),
		strings.Replace(valid, "MainPID=0\n", "", 1),
		valid + "ActiveState=inactive\n", valid + "Unknown=0\n",
		strings.ReplaceAll(valid, "\n", "\r\n"), strings.Repeat(valid, 100),
	} {
		if err := validateRecoveryDormant([]byte(data)); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe service state admitted", data, err)
		}
	}
}

func TestRecoveryDormancyBindsInstalledUnitIdentity(t *testing.T) {
	unit := "homenode-control.service"
	valid := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
	if err := validateRecoveryDormantUnit([]byte(valid), unit); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(valid, "Id="+unit, "Id=foreign.service", 1),
		strings.Replace(valid, "/etc/systemd/system/", "/run/systemd/system/", 1),
		strings.Replace(valid, "DropInPaths=", "DropInPaths=/etc/foreign.conf", 1),
		strings.Replace(valid, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1),
		strings.Replace(valid, "Transient=no", "Transient=yes", 1),
		strings.Replace(valid, "Job=\n", "Job=42\n", 1),
		strings.Replace(valid, "Job=\n", "", 1),
	} {
		if err := validateRecoveryDormantUnit([]byte(data), unit); !errors.Is(err, ErrConflict) {
			t.Fatal("foreign loaded unit admitted", data, err)
		}
	}
	if err := validateRecoveryDormantUnit([]byte(valid), "foreign.service"); !errors.Is(err, ErrPlan) {
		t.Fatal("foreign unit selected", err)
	}
}

func TestRecoveryCredentialSocketMustBeInactive(t *testing.T) {
	unit := "homenode-backup-credential.socket"
	valid := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\nActiveState=inactive\nSubState=dead\n"
	if err := validateRecoveryDormantUnit([]byte(valid), unit); err != nil {
		t.Fatal(err)
	}
	listening := strings.Replace(strings.Replace(valid, "ActiveState=inactive", "ActiveState=active", 1), "SubState=dead", "SubState=listening", 1)
	if err := validateRecoveryDormantUnit([]byte(listening), unit); !errors.Is(err, ErrConflict) {
		t.Fatal("listening credential socket admitted", err)
	}
}
