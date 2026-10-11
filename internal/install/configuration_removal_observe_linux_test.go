//go:build linux

package install

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestConfigurationRemovalObserverBindsManagerAbsenceToJournal(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	ctx := context.Background()
	unit := "homenode-gateway.service"
	plan := fixturePlan()
	plan.Items = append(plan.Items, Item{Path: "etc/systemd/system/" + unit, Mode: 0644, UID: os.Geteuid(), GID: os.Getegid(), Data: []byte("owned unit")})
	if err := e.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareConfigurationRemovalLocked(ctx); err != nil {
		t.Fatal(err)
	}
	queries := 0
	absent := false
	command := func(ctx context.Context, name string, args ...string) *exec.Cmd {

		if name == "/usr/bin/busctl" {
			response := `{"type":"a(sbbsi)","data":[["ConditionPathExists",false,true,"/var/lib/homenode-install/recovery-blocked",0]]}`
			return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '%s' \"$1\"", "fixture", response)
		}
		queries++
		if name != "/usr/bin/systemctl" || len(args) != 6 || args[5] != unit {
			t.Fatal(name, args)
		}
		response := "Id=" + unit + "\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\nTransient=no\nJob=\nLoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
		if absent {
			response = strings.Replace(strings.Replace(response, "FragmentPath=/etc/systemd/system/"+unit, "FragmentPath=", 1), "LoadState=loaded", "LoadState=not-found", 1)
		}
		return exec.CommandContext(ctx, "/bin/sh", "-c", "printf '%s' \"$1\"", "fixture", response)
	}
	if err := e.observeConfigurationRemovalManagerWith(ctx, command); err != nil || queries != 1 {
		t.Fatal(err, queries)
	}
	absent = true
	if err := e.observeConfigurationRemovalManagerWith(ctx, command); err == nil {
		t.Fatal("manager absence adopted with live fragment")
	}
	if err := e.host.Remove("etc/systemd/system/" + unit); err != nil {
		t.Fatal(err)
	}
	before := queries
	if err := e.observeConfigurationRemovalManagerWith(ctx, command); err == nil || queries != before {
		t.Fatal("future missing fragment queried manager", err, queries)
	}
}
