//go:build linux

package updates

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ReadInspectionJournalResult reads only the fixed local unit's retained boot
// and invocation. The coordinator must first verify recorded completion while
// holding operation admission. This helper grants no installation authority.
func ReadInspectionJournalResult(ctx context.Context, expected InspectionIdentity, execution InspectionExecution) (InspectionResult, error) {
	if os.Geteuid() != 0 {
		return InspectionResult{}, ErrInspectionResult
	}
	return readInspectionJournalResultWith(ctx, expected, execution, exec.CommandContext)
}

func readInspectionJournalResultWith(ctx context.Context, expected InspectionIdentity, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) (InspectionResult, error) {
	var zero InspectionResult
	if !validInspectionIdentity(expected) || !validInspectionInvocation(execution.InvocationID) {
		return zero, ErrInspectionResult
	}
	if err := VerifyInspectionLaunchEpoch(ctx, execution.Epoch); err != nil {
		return zero, err
	}
	boot := strings.ReplaceAll(execution.Epoch.BootID, "-", "")
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := command(bounded, "/usr/bin/journalctl", "--system", "--no-pager", "--quiet", "--all", "--output=json", "--lines=2", "--output-fields=MESSAGE,_SYSTEMD_UNIT,_SYSTEMD_INVOCATION_ID,_BOOT_ID,_TRANSPORT,_LINE_BREAK", "--", "_SYSTEMD_UNIT=homenode-inspect.service", "_SYSTEMD_INVOCATION_ID="+execution.InvocationID, "_BOOT_ID="+boot)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	output := &inspectionJournalOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return zero, errors.Join(ErrInspectionResult, err, bounded.Err())
	}
	if err := bounded.Err(); err != nil {
		return zero, err
	}
	if err := VerifyInspectionLaunchEpoch(ctx, execution.Epoch); err != nil {
		return zero, err
	}
	return ValidateInspectionJournalEntry(output.Bytes(), expected, execution.InvocationID, boot)
}

type inspectionJournalOutput struct{ bytes.Buffer }

func (b *inspectionJournalOutput) Write(data []byte) (int, error) {
	if len(data) > 8192-b.Len() {
		return 0, ErrInspectionResult
	}
	return b.Buffer.Write(data)
}
