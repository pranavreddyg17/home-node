//go:build linux

package updates

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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
	if _, err := captureInspectionServiceInvocationWith(context.Background(), 0, factory); err == nil || called {
		t.Fatal("missing capture boundary reached manager")
	}
	if _, err := captureInspectionServiceInvocationWith(ctx, 1, factory); err == nil || called {
		t.Fatal("canceled capture reached manager")
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
	properties := "InvocationID=" + invocation + "\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=active\nSubState=exited\nExecMainStartTimestampMonotonic=200\nExecMainExitTimestampMonotonic=300\n"
	var launched *exec.Cmd
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path != "/usr/bin/systemctl" || len(args) != 5 || args[0] != "--system" || args[1] != "--no-pager" || args[2] != "show" || args[3] != "--property=InvocationID,Result,ExecMainCode,ExecMainStatus,ActiveState,SubState,ExecMainStartTimestampMonotonic,ExecMainExitTimestampMonotonic" || args[4] != "homenode-inspect.service" {
			t.Fatal("unexpected manager scope", path, args)
		}
		launched = exec.CommandContext(ctx, "/usr/bin/printf", "%s", properties)
		return launched
	}
	if err := verifyInspectionServiceCompletionWith(context.Background(), invocation, 100, factory); err != nil {
		t.Fatal(err)
	}
	captured, err := captureInspectionServiceInvocationWith(context.Background(), 100, factory)
	if err != nil || captured != invocation {
		t.Fatal("manager invocation capture failed", captured, err)
	}
	if strings.Join(launched.Env, "\n") != "PATH=/usr/bin:/bin\nLC_ALL=C\nSYSTEMD_COLORS=0\nSYSTEMD_PAGER=cat" {
		t.Fatal("inherited manager environment", launched.Env)
	}
}

func TestInspectionManagerProcessHelper(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "inspection-manager-helper" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "failed":
		fmt.Print("InvocationID=" + strings.Repeat("a", 32) + "\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=active\nSubState=exited\nExecMainStartTimestampMonotonic=200\nExecMainExitTimestampMonotonic=300\n")
		os.Exit(3)
	case "oversized":
		fmt.Print(strings.Repeat("x", 4096))
		os.Exit(0)
	case "malformed":
		fmt.Print("Result=success\n")
		os.Exit(0)
	case "blocked":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(4)
}

func TestInspectionManagerRejectsFailedOversizedMalformedAndCanceledProcesses(t *testing.T) {
	for _, mode := range []string{"failed", "oversized", "malformed", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			if mode == "blocked" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			factory := func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInspectionManagerProcessHelper$", "--", "inspection-manager-helper", mode)
			}
			err := verifyInspectionServiceCompletionWith(ctx, strings.Repeat("a", 32), 100, factory)
			if err == nil {
				t.Fatal("unsafe manager process accepted")
			}
			if mode == "blocked" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("deadline identity lost", err)
			}
		})
	}
}

func TestInspectionSyscallManagerQualifiedProfile(t *testing.T) {
	called := false
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		called = true
		if path != "/usr/bin/systemctl" || strings.Join(args, " ") != "--system --no-pager show --property=SystemCallFilter homenode-inspect.service" {
			t.Fatal("unexpected syscall manager scope", path, args)
		}
		return exec.CommandContext(ctx, "/usr/bin/printf", "%s", "SystemCallFilter=~reboot mount\n")
	}
	for _, profile := range [][]string{nil, {"mount", "mount"}, {"@mount"}, {strings.Repeat("a", 4096)}} {
		if err := verifyInspectionSyscallFilterWith(context.Background(), profile, factory); err == nil || called {
			t.Fatal("unqualified profile reached manager")
		}
	}
	if err := verifyInspectionSyscallFilterWith(context.Background(), []string{"mount", "reboot"}, factory); err != nil {
		t.Fatal(err)
	}
	if err := verifyInspectionSyscallFilterWith(context.Background(), []string{"mount"}, factory); err == nil {
		t.Fatal("unexpected manager denyset accepted")
	}
}
