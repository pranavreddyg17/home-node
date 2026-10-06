//go:build linux

package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// ObserveRecoveryServicesDormant checks fixed service observations only. It
// does not stop services, mask activation, prove empty guest cgroups, or grant
// publication authority. A retained activation barrier is required separately.
func ObserveRecoveryServicesDormant(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return ErrConflict
	}
	return observeRecoveryServicesWith(ctx, exec.CommandContext)
}

func observeRecoveryServicesWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil {
		return ErrPlan
	}
	bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for _, unit := range []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket"} {
		if err := bounded.Err(); err != nil {
			return err
		}
		step, finish := context.WithTimeout(bounded, 5*time.Second)
		properties := "--property=Id,FragmentPath,DropInPaths,NeedDaemonReload,Transient,LoadState,ActiveState,SubState"
		if unit != "homenode-backup-credential.socket" {
			properties += ",MainPID,ControlPID"
		}
		cmd := command(step, "/usr/bin/systemctl", "--system", "--no-pager", "show", properties, unit)
		if cmd == nil {
			finish()
			return ErrPlan
		}
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
		output := &recoveryManagerOutput{}
		cmd.Stdout = output
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		contextErr := step.Err()
		finish()
		if err != nil || contextErr != nil {
			return errors.Join(ErrConflict, err, contextErr)
		}
		if err = validateRecoveryDormantUnit(output.Bytes(), unit); err != nil {
			return err
		}
	}
	return bounded.Err()
}

type recoveryManagerOutput struct{ bytes.Buffer }

func (b *recoveryManagerOutput) Write(data []byte) (int, error) {
	if len(data) > 1024-b.Len() {
		return 0, ErrConflict
	}
	return b.Buffer.Write(data)
}
