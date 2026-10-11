//go:build linux

package install

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestConfigurationRemovalStopPhasesRetainGuardAndEmptyGuests(t *testing.T) {
	plan := ConfigurationRemovalPlan{Files: []ConfigurationRemovalFile{{Path: "etc/systemd/system/homenode.slice"}, {Path: "etc/systemd/system/homenode-inspect.service"}, {Path: "etc/systemd/system/homenode-app-maintenance.socket"}, {Path: "etc/systemd/system/homenode-gateway.service"}}}
	var calls [][]string
	command := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/usr/bin/systemctl" {
			t.Fatal(name)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("unbounded stop")
		}
		calls = append(calls, args)
		return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
	}
	guests := 0
	guard := func(context.Context) error { return nil }
	empty := func(context.Context) error { guests++; return nil }
	if err := stopConfigurationRemovalUnitsWith(context.Background(), command, plan, guard, empty); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"--system", "--no-pager", "--no-ask-password", "stop", "homenode-app-maintenance.socket"}, {"--system", "--no-pager", "--no-ask-password", "stop", "homenode-gateway.service", "homenode-inspect.service"}, {"--system", "--no-pager", "--no-ask-password", "stop", "homenode.slice"}}
	if !reflect.DeepEqual(calls, want) || guests != 3 {
		t.Fatal(calls, guests)
	}
	denied := errors.New("guest remains")
	calls = nil
	if err := stopConfigurationRemovalUnitsWith(context.Background(), command, plan, guard, func(context.Context) error { return denied }); !errors.Is(err, denied) || len(calls) != 0 {
		t.Fatal("occupied guest subtree stopped", err, calls)
	}
	calls = nil
	checks := 0
	loss := func(context.Context) error {
		checks++
		if checks == 3 {
			return denied
		}
		return nil
	}
	if err := stopConfigurationRemovalUnitsWith(context.Background(), command, plan, loss, empty); !errors.Is(err, denied) || len(calls) != 1 {
		t.Fatal("guard loss continued later stop phases", err, calls)
	}
}
