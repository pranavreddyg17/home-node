//go:build linux

package updates

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNativeInspectionManagerCompletion(t *testing.T) {
	if os.Getenv("HOMENODE_INSPECT_COMPLETION_INTEGRATION") != "1" || os.Geteuid() != 0 {
		t.Skip("requires opted-in disposable Linux system manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	unit := "homenode-inspect-completion-fixture.service"
	path := "/run/systemd/system/" + unit
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	data := "[Unit]\nDescription=Disposable inspection completion fixture\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/usr/bin/sleep 1\n"
	if _, err := file.WriteString(data); err != nil {
		file.Close()
		os.Remove(path)
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		t.Fatal(err)
	}
	manager := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		return cmd.CombinedOutput()
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "stop", unit).Run()
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "reset-failed", unit).Run()
		os.Remove(path)
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "daemon-reload").Run()
	}()
	if output, err := manager("daemon-reload"); err != nil {
		t.Fatal(string(output), err)
	}
	epoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	boundary := epoch.NotBeforeMicros
	// The seam changes only the disposable unit; executable, scope, environment,
	// bounded output and completion parsing follow the production manager path.
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		copied := append([]string(nil), args...)
		if copied[len(copied)-1] != "homenode-inspect.service" {
			t.Fatal("unexpected production unit")
		}
		copied[len(copied)-1] = unit
		return exec.CommandContext(ctx, path, copied...)
	}
	if err := verifyInspectionDormantWith(ctx, factory); err != nil {
		t.Fatal("fresh loaded fixture was not dormant", err)
	}
	if output, err := manager("start", "--no-block", unit); err != nil {
		t.Fatal(string(output), err)
	}
	var invocation string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		captured, err := captureInspectionExecutionWith(ctx, epoch, factory)
		if err == nil {
			invocation = captured.InvocationID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(invocation) != 32 {
		t.Fatal("manager never published invocation")
	}
	if err := verifyInspectionServiceCompletionWith(ctx, invocation, boundary, factory); err == nil {
		t.Fatal("running fixture accepted as completed")
	}
	var completionErr error
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		completionErr = verifyInspectionServiceCompletionWith(ctx, invocation, boundary, factory)
		if completionErr == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if completionErr != nil {
		output, _ := manager("show", unit)
		t.Fatal("completed invocation refused", completionErr, string(output))
	}
	if err := verifyInspectionExecutionCompletionWith(ctx, InspectionExecution{Epoch: epoch, InvocationID: invocation}, factory); err != nil {
		t.Fatal("boot-bound native completion refused", err)
	}
	if err := verifyInspectionDormantWith(ctx, factory); err == nil {
		t.Fatal("retained completed invocation admitted as dormant")
	}
	if err := verifyInspectionServiceCompletionWith(ctx, strings.Repeat("b", 32), boundary, factory); err == nil {
		t.Fatal("unrelated invocation accepted")
	}
	// Reconfigure only this owned disposable fixture for a failed invocation.
	// A new invocation ID is insufficient when the manager reports failure.
	if output, err := manager("stop", unit); err != nil {
		t.Fatal(string(output), err)
	}
	if err := verifyInspectionDormantWith(ctx, factory); err != nil {
		t.Fatal("explicitly stopped fixture was not dormant", err)
	}
	failedUnit := "[Unit]\nDescription=Disposable failed inspection completion fixture\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/usr/bin/false\n"
	if err := os.WriteFile(path, []byte(failedUnit), 0644); err != nil {
		t.Fatal(err)
	}
	if output, err := manager("daemon-reload"); err != nil {
		t.Fatal(string(output), err)
	}
	failedBoundary, err := InspectionLaunchBoundary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := manager("start", unit); err == nil {
		t.Fatal("failed fixture start reported success", string(output))
	}
	output, err := manager("show", "--property=InvocationID", "--value", unit)
	if err != nil {
		t.Fatal(string(output), err)
	}
	failedInvocation := strings.TrimSpace(string(output))
	if !validInspectionInvocation(failedInvocation) || failedInvocation == invocation {
		t.Fatal("failed fixture lacked fresh invocation", failedInvocation)
	}
	if err := verifyInspectionServiceCompletionWith(ctx, failedInvocation, failedBoundary, factory); err == nil {
		t.Fatal("failed real invocation accepted")
	}
	if err := verifyInspectionDormantWith(ctx, factory); err == nil {
		t.Fatal("failed invocation admitted as dormant")
	}

}
