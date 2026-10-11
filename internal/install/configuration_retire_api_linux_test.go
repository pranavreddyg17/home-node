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

func TestConfigurationRetirementReloadIsBoundedAndRequiresPostcondition(t *testing.T) {
	calls := 0
	beforeCalls := 0
	afterCalls := 0
	command := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls++
		deadline, ok := ctx.Deadline()
		if name != "/usr/bin/systemctl" || !reflect.DeepEqual(args, []string{"--system", "--no-pager", "--no-ask-password", "daemon-reload"}) || !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal(name, args)
		}
		return exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
	}
	before := func(context.Context) error { beforeCalls++; return nil }
	denied := errors.New("unit remains loaded")
	after := func(context.Context) error { afterCalls++; return denied }
	if err := reloadConfigurationRemovalManagerWith(context.Background(), command, before, after); !errors.Is(err, denied) || calls != 1 || beforeCalls != 1 || afterCalls != 1 {
		t.Fatal("command success replaced postcondition", err, calls, beforeCalls, afterCalls)
	}
	if err := reloadConfigurationRemovalManagerWith(context.Background(), command, func(context.Context) error { return denied }, after); !errors.Is(err, denied) || calls != 1 {
		t.Fatal("failed guard allowed reload", err, calls)
	}
}
