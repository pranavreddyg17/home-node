//go:build linux

package updates

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
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
	if !validInspectionInvocation(invocation) || notBeforeMicros == 0 {
		return ErrInspectionResult
	}
	properties, err := inspectionServiceProperties(ctx, command)
	if err != nil {
		return err
	}
	return ValidateInspectionCompletion(properties, invocation, notBeforeMicros)
}

// CaptureInspectionServiceInvocation queries the fixed local inspection unit
// and retains only a fresh running or successfully completed invocation.
func CaptureInspectionServiceInvocation(ctx context.Context, notBeforeMicros uint64) (string, error) {
	if os.Geteuid() != 0 {
		return "", ErrInspectionResult
	}
	return captureInspectionServiceInvocationWith(ctx, notBeforeMicros, exec.CommandContext)
}

func captureInspectionServiceInvocationWith(ctx context.Context, notBeforeMicros uint64, command func(context.Context, string, ...string) *exec.Cmd) (string, error) {
	if notBeforeMicros == 0 {
		return "", ErrInspectionResult
	}
	properties, err := inspectionServiceProperties(ctx, command)
	if err != nil {
		return "", err
	}
	return InspectionInvocationFromManager(properties, notBeforeMicros)
}

func inspectionServiceProperties(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd) ([]byte, error) {
	return inspectionManagerQuery(ctx, "--property=InvocationID,Result,ExecMainCode,ExecMainStatus,ActiveState,SubState,ExecMainStartTimestampMonotonic,ExecMainExitTimestampMonotonic", command)
}

// VerifyInspectionUnitIdentity checks manager load identity for the owned unit;
// it is not a complete effective-confinement check or activation authorization.
func VerifyInspectionUnitIdentity(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	properties, err := inspectionManagerQuery(ctx, "--property=Id,LoadState,FragmentPath,DropInPaths,NeedDaemonReload,Type,RemainAfterExit,DynamicUser,Transient", exec.CommandContext)
	if err != nil {
		return err
	}
	return ValidateInspectionUnitIdentity(properties)
}

func inspectionManagerQuery(ctx context.Context, propertyFlag string, command func(context.Context, string, ...string) *exec.Cmd) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := command(bounded, "/usr/bin/systemctl", "--system", "--no-pager", "show", propertyFlag, "homenode-inspect.service")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	output := &inspectionManagerOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return nil, errors.Join(ErrInspectionResult, err, bounded.Err())
	}
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

type inspectionManagerOutput struct{ bytes.Buffer }

func (b *inspectionManagerOutput) Write(data []byte) (int, error) {
	if len(data) > 2048-b.Len() {
		return 0, ErrInspectionResult
	}
	return b.Buffer.Write(data)
}

// VerifyInspectionResources queries configured manager limits. It is an
// activation prerequisite, not proof of live cgroup enforcement.
func VerifyInspectionResources(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	properties, err := inspectionManagerQuery(ctx, "--property=MemoryMax,MemorySwapMax,CPUQuotaPerSecUSec,TasksMax,OOMPolicy,KillMode,Restart,TimeoutStartUSec,TimeoutStopUSec", exec.CommandContext)
	if err != nil {
		return err
	}
	return ValidateInspectionResources(properties)
}

// VerifyInspectionIsolation queries core manager-configured confinement. More
// effective policy and live enforcement checks are required before activation.
func VerifyInspectionIsolation(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	properties, err := inspectionManagerQuery(ctx, "--property=NoNewPrivileges,CapabilityBoundingSet,AmbientCapabilities,ProtectSystem,ProtectHome,PrivateTmp,PrivateDevices,PrivateNetwork,ProtectKernelTunables,ProtectKernelModules,ProtectKernelLogs,ProtectControlGroups,ProtectProc,ProcSubset,RestrictSUIDSGID,RestrictRealtime,LockPersonality,UMask,SupplementaryGroups", exec.CommandContext)
	if err != nil {
		return err
	}
	return ValidateInspectionIsolation(properties)
}
