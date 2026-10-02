//go:build linux

package updates

import (
	"bytes"
	"context"
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
	identity := InspectionIdentity{OperationID: "inspection-journal-fixture-0001", Release: "0.1.0", PackageSHA256: strings.Repeat("ab", 32), PackageLength: 12}
	message, err := json.Marshal(InspectionResult{Schema: 1, OperationID: identity.OperationID, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: 12, ContentValid: true})
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
	manager("start", unit)
	execution, err := CaptureInspectionExecution(ctx, epoch)
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
	manager("stop", unit)
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
}
