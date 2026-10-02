//go:build linux

package updates

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestInspectionExecutionRequiresRetainedEpochBeforeManagerEffects(t *testing.T) {
	ctx := context.Background()
	epoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	invocation := strings.Repeat("a", 32)
	calls := 0
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		calls++
		properties := fmt.Sprintf("InvocationID=%s\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=active\nSubState=exited\nExecMainStartTimestampMonotonic=%d\nExecMainExitTimestampMonotonic=%d\n", invocation, epoch.NotBeforeMicros, epoch.NotBeforeMicros+1)
		return exec.CommandContext(ctx, "/usr/bin/printf", "%s", properties)
	}
	execution, err := captureInspectionExecutionWith(ctx, epoch, factory)
	if err != nil || execution.Epoch != epoch || execution.InvocationID != invocation {
		t.Fatal("boot-bound capture refused", execution, err)
	}
	if err := verifyInspectionExecutionCompletionWith(ctx, execution, factory); err != nil {
		t.Fatal(err)
	}
	calls = 0
	for _, invalid := range []InspectionLaunchEpoch{{}, {BootID: epoch.BootID, NotBeforeMicros: ^uint64(0)}, {BootID: "00000000-0000-0000-0000-000000000001", NotBeforeMicros: epoch.NotBeforeMicros}} {
		if invalid.BootID == epoch.BootID {
			invalid.BootID = "00000000-0000-0000-0000-000000000002"
		}
		if captured, err := captureInspectionExecutionWith(ctx, invalid, factory); err == nil || captured != (InspectionExecution{}) || calls != 0 {
			t.Fatal("invalid epoch reached manager", captured, err, calls)
		}
		if err := verifyInspectionExecutionCompletionWith(ctx, InspectionExecution{Epoch: invalid, InvocationID: invocation}, factory); err == nil || calls != 0 {
			t.Fatal("invalid completion epoch reached manager", err, calls)
		}
	}
	execution.InvocationID = ""
	if err := verifyInspectionExecutionCompletionWith(ctx, execution, factory); err == nil || calls != 0 {
		t.Fatal("missing invocation reached manager")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if captured, err := captureInspectionExecutionWith(canceled, epoch, factory); err == nil || captured != (InspectionExecution{}) || calls != 0 {
		t.Fatal("canceled execution capture reached manager")
	}
}
