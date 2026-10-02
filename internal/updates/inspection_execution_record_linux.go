//go:build linux

package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

// CaptureAndPublishExecution obtains invocation identity directly from the
// fixed manager unit and durably binds it to the verified launch intent. The
// caller must hold installer admission and must already have requested start.
// Existing or interrupted records require explicit recovery; no service is
// started, stopped, or authorized for installation by this method.
func (s *InspectionStage) CaptureAndPublishExecution(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) (InspectionExecution, error) {
	if os.Geteuid() != 0 {
		return InspectionExecution{}, ErrInspectionResult
	}
	return s.captureAndPublishExecutionOwned(ctx, parent, epoch, exec.CommandContext)
}

func (s *InspectionStage) captureAndPublishExecutionOwned(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch, command func(context.Context, string, ...string) *exec.Cmd) (execution InspectionExecution, resultErr error) {
	var zero InspectionExecution
	if s == nil || parent == nil {
		return zero, ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.verifyLaunchIntentLocked(ctx, parent, epoch); err != nil {
		return zero, err
	}
	for _, name := range []string{"inspection.execution", "inspection.execution.pending"} {
		if _, err := parent.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return zero, errors.Join(ErrInspectionResult, err)
		}
	}
	captured, err := captureInspectionExecutionWith(ctx, epoch, command)
	if err != nil {
		return zero, err
	}
	launch, err := inspectionLaunchRecord(s.identity, epoch)
	if err != nil {
		return zero, err
	}
	data, err := json.Marshal(struct {
		Schema       int             `json:"schema"`
		Launch       json.RawMessage `json:"launch"`
		InvocationID string          `json:"invocationId"`
	}{1, launch, captured.InvocationID})
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := writeInitializationFile(parent, "inspection.execution.pending", data); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	directory, err := parent.Open(".")
	if err != nil {
		return zero, err
	}
	defer func() {
		if err := directory.Close(); err != nil {
			execution = zero
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err := unix.Renameat2(int(directory.Fd()), "inspection.execution.pending", int(directory.Fd()), "inspection.execution", unix.RENAME_NOREPLACE); err != nil {
		return zero, err
	}
	if err := errors.Join(directory.Sync(), ctx.Err()); err != nil {
		return zero, err
	}
	return captured, nil
}
