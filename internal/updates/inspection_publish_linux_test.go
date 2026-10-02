//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInspectionLaunchPublicationBindsParentAndRetainsState(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Chmod(parentPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parentPath, "inspection"), 0700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	root, err := parent.OpenRoot("inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data := []byte("verified launch fixture")
	sourcePath := filepath.Join(t.TempDir(), "source.deb")
	if err := os.WriteFile(sourcePath, data, 0400); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sum := sha256.Sum256(data)
	identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0", PackageSHA256: hex.EncodeToString(sum[:]), PackageLength: int64(len(data))}
	release := &AcquiredRelease{Package: source, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, Metadata: ReleaseMetadata{Release: identity.Release}}
	ctx := context.Background()
	if err := stageInspectionPackageOwned(ctx, root, release, identity.OperationID); err != nil {
		t.Fatal(err)
	}
	stage, err := openInspectionStageOwned(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	unrelatedPath := t.TempDir()
	if err := os.Chmod(unrelatedPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(unrelatedPath, "inspection"), 0700); err != nil {
		t.Fatal(err)
	}
	unrelated, err := os.OpenRoot(unrelatedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unrelated.Close()
	if err := stage.publishEnvironmentOwned(ctx, unrelated); err == nil {
		t.Fatal("unrelated package path configured")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := stage.publishEnvironmentOwned(canceled, parent); err == nil {
		t.Fatal("canceled publication accepted")
	}
	if _, err := os.Lstat(filepath.Join(parentPath, "inspection.env")); !os.IsNotExist(err) {
		t.Fatal("rejected publication changed state", err)
	}
	pendingPath := filepath.Join(parentPath, "inspection.env.pending")
	if err := os.WriteFile(pendingPath, []byte("interrupted launch"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("interrupted launch replaced")
	}
	retained, err := os.ReadFile(pendingPath)
	if err != nil || string(retained) != "interrupted launch" {
		t.Fatal("pending evidence changed", err)
	}
	if _, err := os.Lstat(filepath.Join(parentPath, "inspection.env")); !os.IsNotExist(err) {
		t.Fatal("interrupted launch published", err)
	}
	// Only this disposable fixture explicitly removes interrupted state before
	// exercising a new successful publication.
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal(err)
	}
	expected, err := stage.Environment()
	if err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(filepath.Join(parentPath, "inspection.env"))
	if err != nil || string(published) != string(expected) {
		t.Fatal("launch identity differs", err)
	}
	info, err := os.Lstat(filepath.Join(parentPath, "inspection.env"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("launch file permissions", err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal("published configuration refused", err)
	}
	epoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(canceled, parent, epoch); err == nil {
		t.Fatal("canceled launch intent published")
	}
	if err := stage.publishLaunchIntentOwned(ctx, unrelated, epoch); err == nil {
		t.Fatal("unrelated parent received launch intent")
	}
	wrong := epoch
	wrong.NotBeforeMicros = ^uint64(0)
	if err := stage.publishLaunchIntentOwned(ctx, parent, wrong); err == nil {
		t.Fatal("future launch intent published")
	}
	launchPath := filepath.Join(parentPath, "inspection.launch")
	if _, err := os.Lstat(launchPath); !os.IsNotExist(err) {
		t.Fatal("rejected launch admission changed state", err)
	}
	launchPending := filepath.Join(parentPath, "inspection.launch.pending")
	if err := os.WriteFile(launchPending, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("interrupted launch intent overwritten")
	}
	if retained, err := os.ReadFile(launchPending); err != nil || string(retained) != "interrupted" {
		t.Fatal("interrupted evidence changed", err)
	}
	if err := os.Remove(launchPending); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err != nil {
		t.Fatal(err)
	}
	launch, err := os.ReadFile(launchPath)
	if err != nil || len(launch) == 0 {
		t.Fatal("launch intent missing", err)
	}
	var recorded map[string]json.RawMessage
	wanted := map[string]any{"schema": 1, "operationId": identity.OperationID, "release": identity.Release, "packageSha256": identity.PackageSHA256, "packageLength": identity.PackageLength, "bootId": epoch.BootID, "notBeforeMicros": epoch.NotBeforeMicros}
	if err := json.Unmarshal(launch, &recorded); err != nil || len(recorded) != len(wanted) {
		t.Fatal("launch record shape", err)
	}
	for key, value := range wanted {
		encoded, err := json.Marshal(value)
		if err != nil || string(recorded[key]) != string(encoded) {
			t.Fatal("launch record identity mismatch", key, err)
		}
	}
	if info, err := os.Lstat(launchPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("launch intent permissions", err)
	}
	if err := stage.publishLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("existing launch intent overwritten")
	}
	if retained, err := os.ReadFile(launchPath); err != nil || string(retained) != string(launch) {
		t.Fatal("launch evidence changed on retry", err)
	}
	if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err != nil {
		t.Fatal("valid launch intent refused", err)
	}
	otherEpoch := epoch
	otherEpoch.NotBeforeMicros--
	if err := stage.verifyLaunchIntentOwned(ctx, parent, otherEpoch); err == nil {
		t.Fatal("different retained boundary admitted")
	}
	if err := stage.verifyLaunchIntentOwned(canceled, parent, epoch); err == nil {
		t.Fatal("canceled launch verification admitted")
	}
	for _, altered := range [][]byte{append(append([]byte(nil), launch...), '\n'), []byte(`{"schema":1}`), make([]byte, 2049)} {
		if err := os.WriteFile(launchPath, altered, 0600); err != nil {
			t.Fatal(err)
		}
		if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err == nil {
			t.Fatal("changed launch intent admitted")
		}
	}
	if err := os.WriteFile(launchPath, launch, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(launchPath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("public launch intent admitted")
	}
	if err := os.Chmod(launchPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launchPending, []byte("conflicting"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err == nil {
		t.Fatal("conflicting pending launch intent admitted")
	}
	if err := os.Remove(launchPending); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err != nil {
		t.Fatal("explicitly restored launch intent refused", err)
	}
	// A correct record's bytes do not make a substituted special file or link
	// safe. Preserve the admitted inode while testing each refusal.
	launchRetained := filepath.Join(parentPath, "inspection.launch.retained")
	if err := os.Rename(launchPath, launchRetained); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "symlink", "hardlink", "fifo", "directory"} {
		switch kind {
		case "symlink":
			err = os.Symlink(launchRetained, launchPath)
		case "hardlink":
			err = os.Link(launchRetained, launchPath)
		case "fifo":
			err = unix.Mkfifo(launchPath, 0600)
		case "directory":
			err = os.Mkdir(launchPath, 0700)
		}
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err == nil {
			t.Fatal("unsafe launch intent file admitted", kind)
		}
		if kind != "missing" {
			if err := os.Remove(launchPath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Rename(launchRetained, launchPath); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyLaunchIntentOwned(ctx, parent, epoch); err != nil {
		t.Fatal("restored regular launch intent refused", err)
	}
	calls := 0
	invocation := "0123456789abcdef0123456789abcdef"
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		calls++
		if path != "/usr/bin/systemctl" || len(args) != 5 || args[4] != "homenode-inspect.service" {
			t.Fatal("unexpected execution query scope", path, args)
		}
		properties := fmt.Sprintf("InvocationID=%s\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=active\nSubState=exited\nExecMainStartTimestampMonotonic=%d\nExecMainExitTimestampMonotonic=%d\n", invocation, epoch.NotBeforeMicros, epoch.NotBeforeMicros+1)
		return exec.CommandContext(ctx, "/usr/bin/printf", "%s", properties)
	}
	executionPending := filepath.Join(parentPath, "inspection.execution.pending")
	if err := os.WriteFile(executionPending, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if captured, err := stage.captureAndPublishExecutionOwned(ctx, parent, epoch, factory); err == nil || captured != (InspectionExecution{}) || calls != 0 {
		t.Fatal("interrupted execution record replaced or queried manager", captured, err, calls)
	}
	if err := os.Remove(executionPending); err != nil {
		t.Fatal(err)
	}
	environmentPath := filepath.Join(parentPath, "inspection.env")
	retainedEnvironment, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	changedDuringCapture := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if err := os.WriteFile(environmentPath, []byte("changed during capture"), 0600); err != nil {
			t.Fatal(err)
		}
		return factory(ctx, path, args...)
	}
	if observed, err := stage.captureAndPublishExecutionOwned(ctx, parent, epoch, changedDuringCapture); err == nil || observed != (InspectionExecution{}) || calls != 1 {
		t.Fatal("capture published execution against changed launch inputs", observed, err, calls)
	}
	for _, name := range []string{"inspection.execution", "inspection.execution.pending"} {
		if _, err := parent.Lstat(name); !os.IsNotExist(err) {
			t.Fatal("refused capture retained execution", name, err)
		}
	}
	if err := os.WriteFile(environmentPath, retainedEnvironment, 0600); err != nil {
		t.Fatal(err)
	}
	calls = 0
	captured, err := stage.captureAndPublishExecutionOwned(ctx, parent, epoch, factory)
	if err != nil || captured.Epoch != epoch || captured.InvocationID != invocation || calls != 1 {
		t.Fatal("execution publication failed", captured, err, calls)
	}
	executionPath := filepath.Join(parentPath, "inspection.execution")
	executionData, err := os.ReadFile(executionPath)
	var persisted struct {
		Schema       int             `json:"schema"`
		Launch       json.RawMessage `json:"launch"`
		InvocationID string          `json:"invocationId"`
	}
	if err != nil || json.Unmarshal(executionData, &persisted) != nil || persisted.Schema != 1 || string(persisted.Launch) != string(launch) || persisted.InvocationID != invocation {
		t.Fatal("persisted execution differs from admitted evidence", err)
	}
	if info, err := os.Lstat(executionPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("execution record permissions", err)
	}
	if captured, err := stage.captureAndPublishExecutionOwned(ctx, parent, epoch, factory); err == nil || captured != (InspectionExecution{}) || calls != 1 {
		t.Fatal("existing execution record replaced or queried manager", captured, err, calls)
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err != nil || calls != 2 {
		t.Fatal("recorded completion refused", err, calls)
	}
	wrongExecution := captured
	wrongExecution.InvocationID = "fedcba9876543210fedcba9876543210"
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, wrongExecution, factory); err == nil || calls != 2 {
		t.Fatal("different persisted invocation reached manager", err, calls)
	}
	for _, altered := range [][]byte{[]byte(`{"schema":1}`), append(append([]byte(nil), executionData...), '\n'), make([]byte, 2049)} {
		if err := os.WriteFile(executionPath, altered, 0600); err != nil {
			t.Fatal(err)
		}
		if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err == nil || calls != 2 {
			t.Fatal("altered execution evidence reached manager", err, calls)
		}
	}
	if err := os.WriteFile(executionPath, executionData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executionPending, []byte("conflicting"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err == nil || calls != 2 {
		t.Fatal("conflicting execution evidence reached manager", err, calls)
	}
	if err := os.Remove(executionPending); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err != nil || calls != 3 {
		t.Fatal("restored recorded completion refused", err, calls)
	}
	if err := os.Chmod(executionPath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err == nil || calls != 3 {
		t.Fatal("public execution record reached manager", err, calls)
	}
	if err := os.Chmod(executionPath, 0600); err != nil {
		t.Fatal(err)
	}
	executionRetained := filepath.Join(parentPath, "inspection.execution.retained")
	if err := os.Rename(executionPath, executionRetained); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "symlink", "hardlink", "fifo", "directory"} {
		switch kind {
		case "symlink":
			err = os.Symlink(executionRetained, executionPath)
		case "hardlink":
			err = os.Link(executionRetained, executionPath)
		case "fifo":
			err = unix.Mkfifo(executionPath, 0600)
		case "directory":
			err = os.Mkdir(executionPath, 0700)
		}
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err == nil || calls != 3 {
			t.Fatal("unsafe execution record reached manager", kind, err, calls)
		}
		if kind != "missing" {
			if err := os.Remove(executionPath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Rename(executionRetained, executionPath); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, factory); err != nil || calls != 4 {
		t.Fatal("restored private execution inode refused", err, calls)
	}
	// Correct persisted bytes cannot substitute for a successful current manager
	// observation. Failures here retain all admitted operation evidence.
	for _, output := range []string{
		fmt.Sprintf("InvocationID=%s\nResult=exit-code\nExecMainCode=1\nExecMainStatus=1\nActiveState=failed\nSubState=failed\nExecMainStartTimestampMonotonic=%d\nExecMainExitTimestampMonotonic=%d\n", invocation, epoch.NotBeforeMicros, epoch.NotBeforeMicros+1),
		fmt.Sprintf("InvocationID=%s\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=active\nSubState=exited\nExecMainStartTimestampMonotonic=%d\nExecMainExitTimestampMonotonic=%d\n", "fedcba9876543210fedcba9876543210", epoch.NotBeforeMicros, epoch.NotBeforeMicros+1),
	} {
		failedObservation := func(ctx context.Context, path string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", output)
		}
		if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, failedObservation); err == nil {
			t.Fatal("persisted evidence overrode failed or different manager invocation")
		}
	}
	failedProcess := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/bin/false")
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(ctx, parent, captured, failedProcess); err == nil {
		t.Fatal("manager process failure accepted")
	}
	if err := stage.verifyRecordedExecutionCompletionOwned(canceled, parent, captured, factory); err == nil || calls != 4 {
		t.Fatal("canceled recorded completion reached manager", err, calls)
	}
	if retained, err := os.ReadFile(executionPath); err != nil || string(retained) != string(executionData) {
		t.Fatal("completion refusal altered execution evidence", err)
	}
	resultPayload, err := json.Marshal(InspectionResult{OperationID: identity.OperationID, Schema: 1, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, ContentValid: true})
	if err != nil {
		t.Fatal(err)
	}
	journalPayload, err := json.Marshal(map[string]string{"MESSAGE": string(resultPayload), "_SYSTEMD_UNIT": "homenode-inspect.service", "_SYSTEMD_INVOCATION_ID": invocation, "_BOOT_ID": strings.ReplaceAll(epoch.BootID, "-", ""), "_TRANSPORT": "stdout"})
	if err != nil {
		t.Fatal(err)
	}
	journalCalls := 0
	collectionFactory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path == "/usr/bin/journalctl" {
			journalCalls++
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", string(journalPayload))
		}
		return factory(ctx, path, args...)
	}
	collected, err := stage.collectInspectionResultOwned(ctx, parent, captured, collectionFactory)
	if err != nil || !collected.ContentValid || collected.InstallAuthorized || collected.OperationID != identity.OperationID || journalCalls != 1 || calls != 6 {
		t.Fatal("admitted recorded result collection failed", collected, err, journalCalls, calls)
	}
	validJournalPayload := append([]byte(nil), journalPayload...)
	journalPayload = []byte(`{"MESSAGE":"{}"}`)
	if collected, err := stage.collectInspectionResultOwned(ctx, parent, captured, collectionFactory); err == nil || collected != (InspectionResult{}) || journalCalls != 2 || calls != 7 {
		t.Fatal("invalid journal result admitted", collected, err, journalCalls, calls)
	}
	managerQueries := 0
	postReadFailure := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path == "/usr/bin/journalctl" {
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", string(validJournalPayload))
		}
		managerQueries++
		if managerQueries == 2 {
			return exec.CommandContext(ctx, "/usr/bin/false")
		}
		return factory(ctx, path, args...)
	}
	if collected, err := stage.collectInspectionResultOwned(ctx, parent, captured, postReadFailure); err == nil || collected != (InspectionResult{}) || managerQueries != 2 {
		t.Fatal("valid journal bypassed post-read manager failure", collected, err, managerQueries)
	}
	journalPayload = validJournalPayload
	resultPending := filepath.Join(parentPath, "inspection.result.pending")
	if err := os.WriteFile(resultPending, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	beforeCalls, beforeJournal := calls, journalCalls
	if publishedResult, err := stage.collectAndPublishInspectionResultOwned(ctx, parent, captured, collectionFactory); err == nil || publishedResult != (InspectionResult{}) || calls != beforeCalls || journalCalls != beforeJournal {
		t.Fatal("interrupted result publication queried or replaced evidence", publishedResult, err)
	}
	if retained, err := os.ReadFile(resultPending); err != nil || string(retained) != "interrupted" {
		t.Fatal("interrupted result evidence changed", err)
	}
	if err := os.Remove(resultPending); err != nil {
		t.Fatal(err)
	}
	publishedResult, err := stage.collectAndPublishInspectionResultOwned(ctx, parent, captured, collectionFactory)
	if err != nil || publishedResult != collected || publishedResult.InstallAuthorized || calls != beforeCalls+2 || journalCalls != beforeJournal+1 {
		t.Fatal("durable result collection failed", publishedResult, err)
	}
	resultPath := filepath.Join(parentPath, "inspection.result")
	resultData, err := os.ReadFile(resultPath)
	var persistedResult struct {
		Schema    int              `json:"schema"`
		Execution json.RawMessage  `json:"execution"`
		Result    InspectionResult `json:"result"`
	}
	if err != nil || json.Unmarshal(resultData, &persistedResult) != nil || persistedResult.Schema != 1 || string(persistedResult.Execution) != string(executionData) || persistedResult.Result != collected {
		t.Fatal("retained result differs from admitted execution", err)
	}
	if info, err := os.Lstat(resultPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("result record permissions", err)
	}
	beforeCalls, beforeJournal = calls, journalCalls
	if repeated, err := stage.collectAndPublishInspectionResultOwned(ctx, parent, captured, collectionFactory); err == nil || repeated != (InspectionResult{}) || calls != beforeCalls || journalCalls != beforeJournal {
		t.Fatal("existing result record replaced or recollected", repeated, err)
	}
	if retained, err := os.ReadFile(resultPath); err != nil || string(retained) != string(resultData) {
		t.Fatal("result evidence changed on retry", err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err != nil || recorded != collected || recorded.InstallAuthorized {
		t.Fatal("retained result readback failed", recorded, err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(canceled, parent, captured); err == nil || recorded != (InspectionResult{}) {
		t.Fatal("canceled retained result exposed", recorded, err)
	}
	for _, altered := range [][]byte{[]byte(`{"schema":1}`), append(append([]byte(nil), resultData...), '\n'), make([]byte, 2049)} {
		if err := os.WriteFile(resultPath, altered, 0600); err != nil {
			t.Fatal(err)
		}
		if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err == nil || recorded != (InspectionResult{}) {
			t.Fatal("altered retained result exposed", recorded, err)
		}
	}
	if err := os.WriteFile(resultPath, resultData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPending, []byte("conflicting"), 0600); err != nil {
		t.Fatal(err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err == nil || recorded != (InspectionResult{}) {
		t.Fatal("conflicting retained result exposed", recorded, err)
	}
	if err := os.Remove(resultPending); err != nil {
		t.Fatal(err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err != nil || recorded != collected {
		t.Fatal("restored retained result refused", recorded, err)
	}
	if err := os.Chmod(resultPath, 0644); err != nil {
		t.Fatal(err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err == nil || recorded != (InspectionResult{}) {
		t.Fatal("public retained result exposed", recorded, err)
	}
	if err := os.Chmod(resultPath, 0600); err != nil {
		t.Fatal(err)
	}
	resultRetained := filepath.Join(parentPath, "inspection.result.retained")
	if err := os.Rename(resultPath, resultRetained); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "symlink", "hardlink", "fifo", "directory"} {
		switch kind {
		case "symlink":
			err = os.Symlink(resultRetained, resultPath)
		case "hardlink":
			err = os.Link(resultRetained, resultPath)
		case "fifo":
			err = unix.Mkfifo(resultPath, 0600)
		case "directory":
			err = os.Mkdir(resultPath, 0700)
		}
		if err != nil {
			t.Fatal(kind, err)
		}
		if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err == nil || recorded != (InspectionResult{}) {
			t.Fatal("unsafe retained result exposed", kind, recorded, err)
		}
		if kind != "missing" {
			if err := os.Remove(resultPath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Rename(resultRetained, resultPath); err != nil {
		t.Fatal(err)
	}
	if recorded, err := stage.readRecordedInspectionResultOwned(ctx, parent, captured); err != nil || recorded != collected {
		t.Fatal("restored private result inode refused", recorded, err)
	}
	packageForMutation := filepath.Join(parentPath, "inspection/package.deb")
	writePackageFixture := func(bytes []byte) {
		if err := os.Chmod(packageForMutation, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(packageForMutation, bytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(packageForMutation, 0400); err != nil {
			t.Fatal(err)
		}
	}
	managerQueries = 0
	changedDuringFinalQuery := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if path == "/usr/bin/journalctl" {
			return exec.CommandContext(ctx, "/usr/bin/printf", "%s", string(validJournalPayload))
		}
		managerQueries++
		if managerQueries == 2 {
			changed := append([]byte(nil), data...)
			changed[0] ^= 1
			writePackageFixture(changed)
		}
		return factory(ctx, path, args...)
	}
	if accepted, err := stage.collectInspectionResultOwned(ctx, parent, captured, changedDuringFinalQuery); err == nil || accepted != (InspectionResult{}) || managerQueries != 2 {
		t.Fatal("final-query package mutation exposed result", accepted, err, managerQueries)
	}
	writePackageFixture(data)
	if accepted, err := stage.collectInspectionResultOwned(ctx, parent, captured, collectionFactory); err != nil || accepted != collected {
		t.Fatal("explicitly restored package refused collection", accepted, err)
	}
	packagePath := filepath.Join(parentPath, "inspection/package.deb")
	retainedPath := filepath.Join(parentPath, "inspection/package.retained")
	if err := os.Rename(packagePath, retainedPath); err != nil {
		t.Fatal(err)
	}
	// Even identical bytes in a replacement inode must not be substituted for
	// the descriptor admitted under the operation's execution lock.
	if err := os.WriteFile(packagePath, data, 0400); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("replacement service package inode accepted")
	}
	if err := os.Remove(packagePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(retainedPath, packagePath); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err != nil {
		t.Fatal("restored pinned service path refused", err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, unrelated); err == nil {
		t.Fatal("unrelated launch configuration accepted")
	}
	if err := os.WriteFile(filepath.Join(parentPath, "inspection.env"), []byte("INSPECTION_RELEASE=0.2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("changed configuration accepted")
	}
	if err := os.WriteFile(filepath.Join(parentPath, "inspection.env"), expected, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stage.verifyEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("conflicting pending launch accepted")
	}
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
	}
	if err := stage.publishEnvironmentOwned(ctx, parent); err == nil {
		t.Fatal("existing launch configuration replaced")
	}
	if _, err := os.Lstat(pendingPath); !os.IsNotExist(err) {
		t.Fatal("occupied publication created pending state", err)
	}
}
