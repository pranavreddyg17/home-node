//go:build linux

package updates

import (
	"context"
	"os"
	"os/exec"
)

// InspectionExecution retains manager identity with its independently captured
// boot-bound launch epoch. The coordinator must store it in protected operation
// state; browser and worker claims cannot establish this evidence.
type InspectionExecution struct {
	Epoch        InspectionLaunchEpoch `json:"epoch"`
	InvocationID string                `json:"invocationId"`
}

func CaptureInspectionExecution(ctx context.Context, epoch InspectionLaunchEpoch) (InspectionExecution, error) {
	if os.Geteuid() != 0 {
		return InspectionExecution{}, ErrInspectionResult
	}
	return captureInspectionExecutionWith(ctx, epoch, exec.CommandContext)
}

func captureInspectionExecutionWith(ctx context.Context, epoch InspectionLaunchEpoch, command func(context.Context, string, ...string) *exec.Cmd) (InspectionExecution, error) {
	var zero InspectionExecution
	if err := VerifyInspectionLaunchEpoch(ctx, epoch); err != nil {
		return zero, err
	}
	invocation, err := captureInspectionServiceInvocationWith(ctx, epoch.NotBeforeMicros, command)
	if err != nil {
		return zero, err
	}
	if err := VerifyInspectionLaunchEpoch(ctx, epoch); err != nil {
		return zero, err
	}
	return InspectionExecution{Epoch: epoch, InvocationID: invocation}, nil
}

// VerifyInspectionExecutionCompletion requires both current boot evidence and
// successful fixed-unit completion for the retained manager invocation. It is
// not worker output authentication, durable recovery or installation authority.
func VerifyInspectionExecutionCompletion(ctx context.Context, execution InspectionExecution) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return verifyInspectionExecutionCompletionWith(ctx, execution, exec.CommandContext)
}

func verifyInspectionExecutionCompletionWith(ctx context.Context, execution InspectionExecution, command func(context.Context, string, ...string) *exec.Cmd) error {
	if !validInspectionInvocation(execution.InvocationID) {
		return ErrInspectionResult
	}
	if err := VerifyInspectionLaunchEpoch(ctx, execution.Epoch); err != nil {
		return err
	}
	if err := verifyInspectionServiceCompletionWith(ctx, execution.InvocationID, execution.Epoch.NotBeforeMicros, command); err != nil {
		return err
	}
	return VerifyInspectionLaunchEpoch(ctx, execution.Epoch)
}
