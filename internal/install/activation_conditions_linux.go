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
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil {
		return ErrPlan
	}
	bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for _, unit := range recoveryUnits(requestedGateway(gateway)) {
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
