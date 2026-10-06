package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
)

// ObserveRecoveryQuiescence validates owned activation-conditioned units and
// observes current service dormancy. It neither stops units nor proves empty
// guest cgroups/queued jobs, and must not alone authorize publication.
func (e *Engine) ObserveRecoveryQuiescence(ctx context.Context) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.observeRecoveryQuiescence(ctx, ObserveRecoveryServicesDormant)
}

func (e *Engine) observeRecoveryQuiescence(ctx context.Context, observe func(context.Context) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if observe == nil {
		return ErrPlan
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" {
		return ErrConflict
	}
	if err = e.requireRecoveryActivationBlock(ctx); err != nil {
		return err
	}
	markerFile, err := e.journalRoot.OpenFile("recovery-blocked", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, markerFile.Close()) }()
	markerBefore, err := markerFile.Stat()
	if err != nil {
		return err
	}
	for _, record := range installed.Items {
		if err = e.matches(record); err != nil {
			return ErrConflict
		}
	}
	for _, unit := range []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket"} {
		data, err := e.readConfiguration(installed, "etc/systemd/system/"+unit)
		if err != nil || bytes.Count(data, []byte("ConditionPathExists=!/var/lib/homenode-install/recovery-blocked\n")) != 1 {
			return ErrConflict
		}
	}
	if err = observe(ctx); err != nil {
		return err
	}
	// Recheck marker after manager observation; never recreate lost exclusion.
	if err = e.requireRecoveryActivationBlock(ctx); err != nil {
		return err
	}
	markerAfter, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(markerBefore, markerAfter) {
		return ErrConflict
	}
	return ctx.Err()
}
