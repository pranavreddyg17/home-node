//go:build linux

package updates

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestInspectionManagerPreflightAndOutputBound(t *testing.T) {
	called := false
	factory := func(context.Context, string, ...string) *exec.Cmd { called = true; return nil }
	for _, invocation := range []string{"", strings.Repeat("0", 32), strings.Repeat("A", 32), strings.Repeat("a", 31), "bad;command"} {
		if err := verifyInspectionServiceCompletionWith(context.Background(), invocation, 1, factory); err == nil || called {
			t.Fatal("invalid invocation reached manager")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyInspectionServiceCompletionWith(ctx, strings.Repeat("a", 32), 1, factory); err == nil || called {
		t.Fatal("canceled check reached manager")
	}
	output := &inspectionManagerOutput{}
	if count, err := output.Write(make([]byte, 2048)); err != nil || count != 2048 {
		t.Fatal("bounded output refused", err)
	}
	if _, err := output.Write([]byte("x")); err == nil || output.Len() != 2048 {
		t.Fatal("oversized manager output retained")
	}
}

func TestInspectionManagerCommandUsesFixedLocalScope(t *testing.T) {
	invocation := strings.Repeat("a", 32)
	properties := "InvocationID=" + invocation + "\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=inactive\nSubState=dead\nExecMainStartTimestampMonotonic=200\nExecMainExitTimestampMonotonic=300\n"
	var launched *exec.Cmd
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path != "/usr/bin/systemctl" || len(args) != 5 || args[0] != "--system" || args[1] != "--no-pager" || args[2] != "show" || args[4] != "homenode-inspect.service" {
			t.Fatal("unexpected manager scope", path, args)
		}
		launched = exec.CommandContext(ctx, "/usr/bin/printf", "%s", properties)
		return launched
	}
	if err := verifyInspectionServiceCompletionWith(context.Background(), invocation, 100, factory); err != nil {
		t.Fatal(err)
	}
	if strings.Join(launched.Env, "\n") != "PATH=/usr/bin:/bin\nLC_ALL=C\nSYSTEMD_COLORS=0\nSYSTEMD_PAGER=cat" {
		t.Fatal("inherited manager environment", launched.Env)
	}
}
