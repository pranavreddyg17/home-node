//go:build linux

package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
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
	// Manager capture can wait. Rebind protected launch inputs and package before
	// recording the captured invocation against that admission.
	if err := s.verifyLaunchIntentLocked(ctx, parent, epoch); err != nil {
		return zero, err
	}
	data, err := inspectionExecutionRecord(s.identity, captured)
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

func inspectionExecutionRecord(identity InspectionIdentity, execution InspectionExecution) ([]byte, error) {
	if !validInspectionInvocation(execution.InvocationID) {
		return nil, ErrInspectionResult
	}
	launch, err := inspectionLaunchRecord(identity, execution.Epoch)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Schema       int             `json:"schema"`
		Launch       json.RawMessage `json:"launch"`
		InvocationID string          `json:"invocationId"`
	}{1, launch, execution.InvocationID})
}

// VerifyRecordedExecutionCompletion binds completion evidence to the private
// persisted invocation and admitted launch/package under the stage lock.
// It does not authenticate worker output or grant installation authority.
func (s *InspectionStage) VerifyRecordedExecutionCompletion(ctx context.Context, parent *os.Root, execution InspectionExecution) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return s.verifyRecordedExecutionCompletionOwned(ctx, parent, execution, exec.CommandContext)
}

func (s *InspectionStage) verifyRecordedExecutionCompletionOwned(ctx context.Context, parent *os.Root, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) error {
	if s == nil || parent == nil {
		return ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.verifyExecutionRecordLocked(ctx, parent, execution); err != nil {
		return err
	}
	if err := verifyInspectionExecutionCompletionWith(ctx, execution, command); err != nil {
		return err
	}
	return s.verifyExecutionRecordLocked(ctx, parent, execution)
}

func (s *InspectionStage) verifyExecutionRecordLocked(ctx context.Context, parent *os.Root, execution InspectionExecution) error {
	if !validInspectionInvocation(execution.InvocationID) {
		return ErrInspectionResult
	}
	if err := s.verifyLaunchIntentLocked(ctx, parent, execution.Epoch); err != nil {
		return err
	}
	if _, err := parent.Lstat("inspection.execution.pending"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInspectionResult, err)
	}
	file, err := openInspectionFile(parent, "inspection.execution", 0600)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 2049))
	closeErr := file.Close()
	expected, err := inspectionExecutionRecord(s.identity, execution)
	if err != nil || readErr != nil || closeErr != nil || !bytes.Equal(data, expected) {
		return errors.Join(ErrInspectionResult, err, readErr, closeErr)
	}
	return ctx.Err()
}
