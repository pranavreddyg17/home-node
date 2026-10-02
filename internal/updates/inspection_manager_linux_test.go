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

func TestInspectionConfinementStopsAtFirstRefusedObservation(t *testing.T) {
	for _, denyset := range []string{"SystemCallFilter=~mount\n", "SystemCallFilter=~reboot\n"} {
		calls := 0
		factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
			calls++
			if path != "/usr/bin/systemctl" || len(args) != 5 || args[4] != "homenode-inspect.service" {
				t.Fatal("unexpected aggregate manager scope", path, args)
			}
			output := denyset
			if calls == 2 {
				if args[3] != "--property=Id,LoadState,FragmentPath,DropInPaths,NeedDaemonReload,Type,RemainAfterExit,DynamicUser,Transient" {
					t.Fatal("unit identity check bypassed", args)
				}
				output = "Id=substituted.service\n"
			}
			if calls > 2 {
				t.Fatal("refused observation did not stop preflight")
			}
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", output)
		}
		if err := verifyInspectionConfinementWith(context.Background(), []string{"mount"}, factory); err == nil {
			t.Fatal("refused confinement accepted")
		}
		expected := 1
		if strings.Contains(denyset, "~mount") {
			expected = 2
		}
		if calls != expected {
			t.Fatal("unexpected observation count", calls, expected)
		}
	}
	calls := 0
	factory := func(context.Context, string, ...string) *exec.Cmd { calls++; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyInspectionConfinementWith(ctx, []string{"mount"}, factory); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled aggregate preflight reached manager", err, calls)
	}
	if err := verifyInspectionConfinementWith(context.Background(), nil, factory); err == nil || calls != 0 {
		t.Fatal("missing qualified profile reached manager")
	}
}

func TestInspectionConfinementRequiresEveryPolicy(t *testing.T) {
	snapshots := []string{
		"SystemCallFilter=~mount\n",
		"Id=homenode-inspect.service\nLoadState=loaded\nFragmentPath=/etc/systemd/system/homenode-inspect.service\nDropInPaths=\nNeedDaemonReload=no\nType=oneshot\nRemainAfterExit=yes\nDynamicUser=yes\nTransient=no\n",
		"MemoryMax=268435456\nMemorySwapMax=0\nCPUQuotaPerSecUSec=500ms\nTasksMax=32\nOOMPolicy=kill\nKillMode=control-group\nRestart=no\nTimeoutStartUSec=2min 30s\nTimeoutStopUSec=5s\n",
		"NoNewPrivileges=yes\nCapabilityBoundingSet=\nAmbientCapabilities=\nProtectSystem=strict\nProtectHome=yes\nPrivateTmp=yes\nPrivateDevices=yes\nPrivateNetwork=yes\nProtectKernelTunables=yes\nProtectKernelModules=yes\nProtectKernelLogs=yes\nProtectControlGroups=yes\nProtectProc=invisible\nProcSubset=pid\nRestrictSUIDSGID=yes\nRestrictRealtime=yes\nLockPersonality=yes\nUMask=0077\nSupplementaryGroups=\n",
		"RestrictNamespaces=yes\nRestrictAddressFamilies=AF_UNIX\nSystemCallArchitectures=native\nStandardOutput=journal\nStandardError=journal\n",
		"IPAddressDeny=0.0.0.0/0 ::/0\nIPAddressAllow=\nInaccessiblePaths=-/etc/homenode -/var/lib/homenode -/var/lib/homenode-update -/var/lib/homenode-backup -/run/homenode -/run/homenode-transfer\n",
	}
	for refused := -1; refused < len(snapshots); refused++ {
		calls := 0
		factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
			if calls >= len(snapshots) {
				t.Fatal("unexpected extra query")
			}
			output := snapshots[calls]
			if calls == refused {
				output = "Unknown=unsafe\n"
			}
			calls++
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", output)
		}
		err := verifyInspectionConfinementWith(context.Background(), []string{"mount"}, factory)
		if refused < 0 {
			if err != nil || calls != len(snapshots) {
				t.Fatal("complete preflight refused", err, calls)
			}
		} else if err == nil || calls != refused+1 {
			t.Fatal("policy refusal bypassed", refused, err, calls)
		}
	}
}
