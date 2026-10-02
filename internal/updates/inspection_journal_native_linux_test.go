//go:build linux

package updates

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeInspectionJournalResult(t *testing.T) {
	if os.Getenv("HOMENODE_INSPECT_JOURNAL_INTEGRATION") != "1" || os.Geteuid() != 0 {
		t.Skip("requires opted-in disposable Linux journal fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	unit := "homenode-inspect.service"
	for _, base := range []string{"/etc/systemd/system", "/usr/lib/systemd/system", "/run/systemd/system"} {
		if _, err := os.Lstat(filepath.Join(base, unit)); !os.IsNotExist(err) {
			t.Fatal("fixture refuses existing unit", base, err)
		}
	}
	packageBytes := []byte("journal fixture package")
	sum := sha256.Sum256(packageBytes)
	identity := InspectionIdentity{OperationID: "inspection-journal-fixture-0001", Release: "0.1.0", PackageSHA256: hex.EncodeToString(sum[:]), PackageLength: int64(len(packageBytes))}
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
	staging, err := parent.OpenRoot("inspection")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Close()
	sourcePath := filepath.Join(t.TempDir(), "package.deb")
	if err := os.WriteFile(sourcePath, packageBytes, 0400); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	release := &AcquiredRelease{Package: source, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, Metadata: ReleaseMetadata{Release: identity.Release}}
	if err := StageInspectionPackage(ctx, staging, release, identity.OperationID); err != nil {
		t.Fatal(err)
	}
	stage, err := OpenInspectionStage(ctx, staging, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	if err := stage.PublishEnvironment(ctx, parent); err != nil {
		t.Fatal(err)
	}

	message, err := json.Marshal(InspectionResult{Schema: 1, OperationID: identity.OperationID, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, ContentValid: true})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, append(append([]byte(nil), message...), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	path := "/run/systemd/system/" + unit
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	configuration := "[Unit]\nDescription=Disposable journal identity fixture\n[Service]\nType=oneshot\nRemainAfterExit=yes\nDynamicUser=yes\nStandardInput=file:" + input + "\nStandardOutput=journal\nStandardError=journal\nExecStart=/usr/bin/cat\n"
	_, writeErr := file.WriteString(configuration)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		t.Fatal(writeErr, closeErr)
	}
	manager := func(args ...string) {
		command := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "stop", unit).Run()
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "reset-failed", unit).Run()
		os.Remove(path)
		exec.CommandContext(cleanup, "/usr/bin/systemctl", "daemon-reload").Run()
	}()
	manager("daemon-reload")
	epoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.PublishLaunchIntent(ctx, parent, epoch); err != nil {
		t.Fatal(err)
	}
	manager("start", unit)
	execution, err := stage.CaptureAndPublishExecution(ctx, parent, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyInspectionExecutionCompletion(ctx, execution); err != nil {
		t.Fatal(err)
	}
	syncJournal := func() {
		command := exec.CommandContext(ctx, "/usr/bin/journalctl", "--sync")
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
	var observedJournal string
	awaitJournalEntries := func(execution InspectionExecution, expectedCount int) {
		boot := strings.ReplaceAll(execution.Epoch.BootID, "-", "")
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			bounded, stop := context.WithTimeout(ctx, 2*time.Second)
			command := exec.CommandContext(bounded, "/usr/bin/journalctl", "--system", "--no-pager", "--quiet", "--all", "--output=json", "--lines=2", "--output-fields=MESSAGE,_SYSTEMD_UNIT,_SYSTEMD_INVOCATION_ID,_BOOT_ID,_TRANSPORT,_LINE_BREAK", "--", "_SYSTEMD_UNIT="+unit, "_SYSTEMD_INVOCATION_ID="+execution.InvocationID, "_BOOT_ID="+boot)
			command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
			output := &inspectionJournalOutput{}
			command.Stdout = output
			command.Stderr = io.Discard
			command.WaitDelay = time.Second
			err := command.Run()
			stop()
			if err != nil {
				t.Fatal("native journal observation failed", err)
			}
			decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
			count := 0
			for {
				var fields map[string]json.RawMessage
				err := decoder.Decode(&fields)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal("native journal serialization", err)
				}
				var payload string
				if json.Unmarshal(fields["MESSAGE"], &payload) != nil || payload != string(message) {
					t.Fatal("native journal message differs from emitted bytes")
				}
				count++
			}
			if count == expectedCount {
				observedJournal = output.String()
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal("journal did not retain expected message count", expectedCount)
	}
	syncJournal()
	awaitJournalEntries(execution, 1)
	result, err := ReadInspectionJournalResult(ctx, identity, execution)
	if err != nil || !result.ContentValid || result.InstallAuthorized {
		t.Fatal("native journal result refused", result, err, "bounded synthetic fixture journal:", observedJournal)
	}
	wrong := identity
	wrong.PackageSHA256 = strings.Repeat("cd", 32)
	if _, err := ReadInspectionJournalResult(ctx, wrong, execution); err == nil {
		t.Fatal("wrong admitted package accepted")
	}
	retained, err := stage.CollectAndPublishInspectionResult(ctx, parent, execution)
	if err != nil || retained != result {
		t.Fatal("native result collection/publication failed", retained, err)
	}
	manager("stop", unit)
	if err := VerifyInspectionDormant(ctx); err != nil {
		t.Fatal("stopped fixture did not become dormant", err)
	}
	if err := VerifyInspectionExecutionCompletion(ctx, execution); err == nil {
		t.Fatal("stopped fixture retained active completion authority")
	}
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenInspectionStage(ctx, staging, identity)
	if err != nil {
		t.Fatal("stopped native stage could not reopen", err)
	}
	defer reopened.Close()
	readback, err := reopened.ReadRecordedInspectionResult(ctx, parent, execution)
	if err != nil || readback != result {
		t.Fatal("stopped native result readback failed", readback, err)
	}
	duplicate := append(append(append([]byte(nil), message...), '\n'), message...)
	duplicate = append(duplicate, '\n')
	if err := os.WriteFile(input, duplicate, 0600); err != nil {
		t.Fatal(err)
	}
	nextEpoch, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	manager("start", unit)
	next, err := CaptureInspectionExecution(ctx, nextEpoch)
	if err != nil || next.InvocationID == execution.InvocationID {
		t.Fatal("fresh native invocation missing", next, err)
	}
	if err := VerifyInspectionExecutionCompletion(ctx, next); err != nil {
		t.Fatal(err)
	}
	syncJournal()
	awaitJournalEntries(next, 2)
	if _, err := ReadInspectionJournalResult(ctx, identity, next); err == nil {
		t.Fatal("multiple native result messages accepted")
	}
	if result, err := reopened.CollectInspectionResult(ctx, parent, execution); err == nil || result != (InspectionResult{}) {
		t.Fatal("later manager invocation satisfied prior live collection", result, err)
	}
	if result, err := reopened.CollectInspectionResult(ctx, parent, next); err == nil || result != (InspectionResult{}) {
		t.Fatal("unrecorded later execution satisfied collection", result, err)
	}
	if result, err := reopened.ReadRecordedInspectionResult(ctx, parent, next); err == nil || result != (InspectionResult{}) {
		t.Fatal("fresh invocation substituted retained execution", result, err)
	}
	if readback, err := reopened.ReadRecordedInspectionResult(ctx, parent, execution); err != nil || readback != retained {
		t.Fatal("later invocation invalidated independently retained result", readback, err)
	}
}
