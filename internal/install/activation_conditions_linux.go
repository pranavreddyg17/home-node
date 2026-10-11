//go:build linux

package install

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

// ObserveRecoveryActivationConditions reads the running manager's typed guard
// conditions for fixed units. It neither stops units nor retains exclusion.
func ObserveRecoveryActivationConditions(ctx context.Context, gateway ...bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return ErrConflict
	}
	return observeActivationConditionsWith(ctx, exec.CommandContext, gateway...)
}

func observeActivationConditionsWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, gateway ...bool) error {
	return observeActivationConditionsForUnitsWith(ctx, command, recoveryUnits(requestedGateway(gateway)))
}

// The supplied unit subset is selected from canonical removal provenance.
// Reject arbitrary or repeated names before issuing any bus operation.
func observeActivationConditionsForUnitsWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, units []string) error {
	if len(units) > 7 {
		return ErrPlan
	}
	seen := map[string]bool{}
	for _, unit := range units {
		switch unit {
		case "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket", "homenode-app-maintenance.socket":
		default:
			return ErrPlan
		}
		if seen[unit] {
			return ErrPlan
		}
		seen[unit] = true
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil {
		return ErrPlan
	}
	bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for _, unit := range units {
		if err := bounded.Err(); err != nil {
			return err
		}
		// Only these literal unit names are encoded; no user-provided bus path.
		object := "/org/freedesktop/systemd1/unit/" + strings.NewReplacer("-", "_2d", ".", "_2e").Replace(unit)
		step, finish := context.WithTimeout(bounded, 5*time.Second)
		cmd := command(step, "/usr/bin/busctl", "--system", "--no-pager", "--json=short", "--auto-start=no", "--allow-interactive-authorization=no", "--timeout=4", "get-property", "org.freedesktop.systemd1", object, "org.freedesktop.systemd1.Unit", "Conditions")
		if cmd == nil {
			finish()
			return ErrPlan
		}
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
		output := &activationConditionsOutput{}
		cmd.Stdout, cmd.Stderr = output, io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		contextErr := step.Err()
		finish()
		if err != nil || contextErr != nil {
			return errors.Join(ErrConflict, err, contextErr)
		}
		if err := validateActivationConditions(output.Bytes()); err != nil {
			return err
		}
	}
	return bounded.Err()
}

type activationConditionsOutput struct{ bytes.Buffer }

func (b *activationConditionsOutput) Write(data []byte) (int, error) {
	if len(data) > 8192-b.Len() {
		return 0, ErrConflict
	}
	return b.Buffer.Write(data)
}
