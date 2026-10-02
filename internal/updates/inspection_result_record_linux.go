//go:build linux

package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// CollectAndPublishInspectionResult obtains the result itself through recorded
// completion and the protected local journal, then durably retains its binding
// to admitted execution. Caller-supplied result documents are never accepted.
// Existing/interrupted records require recovery; no installation is authorized.
func (s *InspectionStage) CollectAndPublishInspectionResult(ctx context.Context, parent *os.Root, execution InspectionExecution) (InspectionResult, error) {
	if os.Geteuid() != 0 {
		return InspectionResult{}, ErrInspectionResult
	}
	return s.collectAndPublishInspectionResultOwned(ctx, parent, execution, exec.CommandContext)
}

func (s *InspectionStage) collectAndPublishInspectionResultOwned(ctx context.Context, parent *os.Root, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) (result InspectionResult, resultErr error) {
	var zero InspectionResult
	if s == nil || parent == nil {
		return zero, ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return zero, err
	}
	for _, name := range []string{"inspection.result", "inspection.result.pending"} {
		if _, err := parent.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return zero, errors.Join(ErrInspectionResult, err)
		}
	}
	collected, err := s.collectInspectionResultLocked(bounded, parent, execution, command)
	if err != nil {
		return zero, err
	}
	executionData, err := inspectionExecutionRecord(s.identity, execution)
	if err != nil {
		return zero, err
	}
	data, err := json.Marshal(struct {
		Schema    int              `json:"schema"`
		Execution json.RawMessage  `json:"execution"`
		Result    InspectionResult `json:"result"`
	}{1, executionData, collected})
	if err != nil {
		return zero, err
	}
	if err := bounded.Err(); err != nil {
		return zero, err
	}
	if err := writeInitializationFile(parent, "inspection.result.pending", data); err != nil {
		return zero, err
	}
	if err := bounded.Err(); err != nil {
		return zero, err
	}
	directory, err := parent.Open(".")
	if err != nil {
		return zero, err
	}
	defer func() {
		if err := directory.Close(); err != nil {
			result = zero
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err := unix.Renameat2(int(directory.Fd()), "inspection.result.pending", int(directory.Fd()), "inspection.result", unix.RENAME_NOREPLACE); err != nil {
		return zero, err
	}
	if err := errors.Join(directory.Sync(), bounded.Err()); err != nil {
		return zero, err
	}
	return collected, nil
}
