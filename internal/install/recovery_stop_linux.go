//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// QuiesceRecovery stops owned application units behind an existing durable
// activation block and observes guest emptiness. The marker is never released.
// This operation does not return a publication lease or migrate any storage.
func (e *Engine) QuiesceRecovery(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	return e.observeRecoveryQuiescence(ctx, func(ctx context.Context) error {
		return quiesceRecoveryManagerWith(ctx, exec.CommandContext, e.requireRecoveryActivationBlock, ObserveRecoveryGuestsEmpty)
	})
}

func quiesceRecoveryManagerWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd, marker, guests func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil || marker == nil || guests == nil {
		return ErrPlan
	}
	if err := observeRecoveryManagerWith(ctx, command, false); err != nil {
		return err
	}
	if err := observeActivationConditionsWith(ctx, command); err != nil {
		return err
	}
	if err := marker(ctx); err != nil {
		return err
	}
	if err := stopRecoveryServicesWith(ctx, command); err != nil {
		return err
	}
	if err := observeRecoveryServicesWith(ctx, command); err != nil {
		return err
	}
	if err := observeActivationConditionsWith(ctx, command); err != nil {
		return err
	}
	if err := marker(ctx); err != nil {
		return err
	}
	return guests(ctx)
}

// stopRecoveryServicesWith is only the bounded manager operation. A caller
// must retain installer/activation exclusion and qualify loaded unit ownership
// before invoking it, then independently observe dormancy and guest emptiness.
// It never removes an activation marker, restarts units or kills guest PIDs.
func stopRecoveryServicesWith(ctx context.Context, command func(context.Context, string, ...string) *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == nil {
		return ErrPlan
	}
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := command(bounded, "/usr/bin/systemctl", "--system", "--no-pager", "--no-ask-password", "stop", "homenode-backup-credential.socket", "homenode-control.service", "homenode-transfer.service", "homenode-backup.service", "homenode-supervisor.service")
	if cmd == nil {
		return ErrPlan
	}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.Join(ErrConflict, err, bounded.Err())
	}
	return bounded.Err()
}
