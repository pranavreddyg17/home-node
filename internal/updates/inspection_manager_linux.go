//go:build linux

package updates

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// VerifyInspectionServiceCompletion obtains evidence directly from the local
// system manager for the fixed inspection unit. The protected coordinator must
// retain the invocation and pre-launch monotonic boundary independently.
func VerifyInspectionServiceCompletion(ctx context.Context, invocation string, notBeforeMicros uint64) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return verifyInspectionServiceCompletionWith(ctx, invocation, notBeforeMicros, exec.CommandContext)
}

func verifyInspectionServiceCompletionWith(ctx context.Context, invocation string, notBeforeMicros uint64, command func(context.Context, string, ...string) *exec.Cmd) error {
	decoded, err := hex.DecodeString(invocation)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != invocation || invocation == strings.Repeat("0", 32) || notBeforeMicros == 0 {
		return ErrInspectionResult
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := command(bounded, "/usr/bin/systemctl", "--system", "--no-pager", "show", "--property=InvocationID,Result,ExecMainCode,ExecMainStatus,ActiveState,SubState,ExecMainStartTimestampMonotonic,ExecMainExitTimestampMonotonic", "homenode-inspect.service")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	output := &inspectionManagerOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.Join(ErrInspectionResult, err, bounded.Err())
	}
	if err := bounded.Err(); err != nil {
		return err
	}
	return ValidateInspectionCompletion(output.Bytes(), invocation, notBeforeMicros)
}

type inspectionManagerOutput struct{ bytes.Buffer }

func (b *inspectionManagerOutput) Write(data []byte) (int, error) {
	if len(data) > 2048-b.Len() {
		return 0, ErrInspectionResult
	}
	return b.Buffer.Write(data)
}
