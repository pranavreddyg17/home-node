//go:build linux

package updates

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestInspectionJournalQueryUsesFixedLocalContext(t *testing.T) {
	epoch, err := CaptureInspectionLaunchEpoch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expected := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0", PackageSHA256: strings.Repeat("ab", 32), PackageLength: 12}
	execution := InspectionExecution{Epoch: epoch, InvocationID: strings.Repeat("a", 32)}
	message, _ := json.Marshal(InspectionResult{Schema: 1, OperationID: expected.OperationID, Release: expected.Release, PackageSHA256: expected.PackageSHA256, PackageLength: 12, ContentValid: true})
	boot := strings.ReplaceAll(epoch.BootID, "-", "")
	envelope, _ := json.Marshal(map[string]string{"MESSAGE": string(message), "_SYSTEMD_UNIT": "homenode-inspect.service", "_SYSTEMD_INVOCATION_ID": execution.InvocationID, "_BOOT_ID": boot, "_TRANSPORT": "stdout"})
	calls := 0
	var launched *exec.Cmd
	factory := func(ctx context.Context, path string, args ...string) *exec.Cmd {
		calls++
		wanted := []string{"--system", "--no-pager", "--quiet", "--all", "--output=json", "--lines=2", "--output-fields=MESSAGE,_SYSTEMD_UNIT,_SYSTEMD_INVOCATION_ID,_BOOT_ID,_TRANSPORT,_LINE_BREAK", "--", "_SYSTEMD_UNIT=homenode-inspect.service", "_SYSTEMD_INVOCATION_ID=" + execution.InvocationID, "_BOOT_ID=" + boot}
		if path != "/usr/bin/journalctl" || strings.Join(args, "\n") != strings.Join(wanted, "\n") {
			t.Fatal("unexpected journal scope", path, args)
		}
		launched = exec.CommandContext(ctx, "/usr/bin/printf", "%s", string(envelope))
		return launched
	}
	result, err := readInspectionJournalResultWith(context.Background(), expected, execution, factory)
	if err != nil || !result.ContentValid || result.InstallAuthorized {
		t.Fatal(result, err)
	}
	if strings.Join(launched.Env, "\n") != "PATH=/usr/bin:/bin\nLC_ALL=C\nSYSTEMD_COLORS=0\nSYSTEMD_PAGER=cat" {
		t.Fatal("inherited journal environment")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readInspectionJournalResultWith(canceled, expected, execution, factory); err == nil || calls != 1 {
		t.Fatal("canceled journal query reached process")
	}
	bad := execution
	bad.InvocationID = "bad;command"
	if _, err := readInspectionJournalResultWith(context.Background(), expected, bad, factory); err == nil || calls != 1 {
		t.Fatal("invalid invocation reached process")
	}
	buffer := &inspectionJournalOutput{}
	if n, err := buffer.Write(make([]byte, 8192)); n != 8192 || err != nil {
		t.Fatal(n, err)
	}
	if _, err := buffer.Write([]byte("x")); err == nil || buffer.Len() != 8192 {
		t.Fatal("oversized journal output retained")
	}
}
