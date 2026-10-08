//go:build linux

package install

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestActivationConditionsQueriesFixedManagerObjects(t *testing.T) {
	objects := []string{"homenode_2dcontrol_2eservice", "homenode_2dtransfer_2eservice", "homenode_2dsupervisor_2eservice", "homenode_2dbackup_2eservice", "homenode_2dbackup_2dcredential_2esocket"}
	queries := 0
	response := `{"type":"a(sbbsi)","data":[["ConditionPathExists",false,true,"/var/lib/homenode-install/recovery-blocked",0]]}`
	command := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if queries >= len(objects) {
			t.Fatal("extra query")
		}
		want := []string{"--system", "--no-pager", "--json=short", "--auto-start=no", "--allow-interactive-authorization=no", "--timeout=4", "get-property", "org.freedesktop.systemd1", "/org/freedesktop/systemd1/unit/" + objects[queries], "org.freedesktop.systemd1.Unit", "Conditions"}
		if path != "/usr/bin/busctl" || !reflect.DeepEqual(args, want) {
			t.Fatal("unexpected manager query", path, args)
		}
		queries++
		return exec.CommandContext(ctx, "/bin/sh", "-c", `printf '%s' "$1"`, "fixture", response)
	}
	if err := observeActivationConditionsWith(context.Background(), command); err != nil || queries != len(objects) {
		t.Fatal("fixed conditions query", queries, err)
	}
	queries = 0
	response = `{"type":"a(sbbsi)","data":[]}`
	if err := observeActivationConditionsWith(context.Background(), command); !errors.Is(err, ErrConflict) || queries != 1 {
		t.Fatal("missing guard continued", queries, err)
	}
	queries = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observeActivationConditionsWith(ctx, command); !errors.Is(err, context.Canceled) || queries != 0 {
		t.Fatal("cancelled query launched", queries, err)
	}
	if err := observeActivationConditionsWith(context.Background(), nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing adapter admitted", err)
	}
	var output activationConditionsOutput
	if _, err := output.Write(make([]byte, 8193)); !errors.Is(err, ErrConflict) || output.Len() != 0 {
		t.Fatal("unbounded output retained", err)
	}
}
