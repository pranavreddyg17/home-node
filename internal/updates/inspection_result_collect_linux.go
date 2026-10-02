//go:build linux

package updates

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// CollectInspectionResult binds local journal output to protected recorded
// completion and the locked admitted package. The coordinator must retain
// installer/service/policy admission; this grants no installation authority.
func (s *InspectionStage) CollectInspectionResult(ctx context.Context, parent *os.Root, execution InspectionExecution) (InspectionResult, error) {
	if os.Geteuid() != 0 {
		return InspectionResult{}, ErrInspectionResult
	}
	return s.collectInspectionResultOwned(ctx, parent, execution, exec.CommandContext)
}

func (s *InspectionStage) collectInspectionResultOwned(ctx context.Context, parent *os.Root, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) (InspectionResult, error) {
	var zero InspectionResult
	if s == nil || parent == nil {
		return zero, ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.collectInspectionResultLocked(ctx, parent, execution, command)
}

func (s *InspectionStage) collectInspectionResultLocked(ctx context.Context, parent *os.Root, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) (InspectionResult, error) {
	var zero InspectionResult
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.verifyExecutionRecordLocked(bounded, parent, execution); err != nil {
		return zero, err
	}
	if err := verifyInspectionExecutionCompletionWith(bounded, execution, command); err != nil {
		return zero, err
	}
	result, err := readInspectionJournalResultWith(bounded, s.identity, execution, command)
	if err != nil {
		return zero, err
	}
	// Revalidate fixed package/configuration/records and retained invocation after
	// the journal read; a journal message cannot stand in for current completion.
	if err := s.verifyExecutionRecordLocked(bounded, parent, execution); err != nil {
		return zero, err
	}
	if err := verifyInspectionExecutionCompletionWith(bounded, execution, command); err != nil {
		return zero, err
	}
	// The last manager query is itself an interval during which fixed disk state
	// may change. Rebind to the admitted package/records before returning bytes.
	if err := s.verifyExecutionRecordLocked(bounded, parent, execution); err != nil {
		return zero, err
	}
	if err := bounded.Err(); err != nil {
		return zero, err
	}
	return result, nil
}
