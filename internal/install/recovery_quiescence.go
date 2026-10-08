package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"time"
)

// ObserveRecoveryQuiescence validates owned activation-conditioned units and
// observes current service dormancy and guest cgroup emptiness. It neither
// stops units nor excludes queued/future activation, and must not alone
// authorize publication.
func (e *Engine) ObserveRecoveryQuiescence(ctx context.Context) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	return e.observeRecoveryQuiescence(ctx, func(ctx context.Context) error {
		if err := ObserveRecoveryActivationConditions(ctx); err != nil {
			return err
		}
		if err := ObserveRecoveryServicesDormant(ctx); err != nil {
			return err
		}
		return ObserveRecoveryGuestsEmpty(ctx)
	})
}

func (e *Engine) observeRecoveryQuiescence(ctx context.Context, observe func(context.Context) error) (result error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return e.withRecoveryExclusionLocked(ctx, observe, e.observeRecoveryDestinationVacancy, nil)
}

// withRecoveryExclusionLocked retains the marker descriptor and installation
// authority across a consumer. Caller must hold e.mu. Destination admission is
// separate so a journaled publisher can validate its own exact retry artifacts
// rather than treating its newly published files as foreign occupied storage.
// No production publisher is enabled by this composition boundary alone.
func (e *Engine) withRecoveryExclusionLocked(ctx context.Context, observe, destinations, use func(context.Context) error) (result error) {
	var guarded func(context.Context, func(context.Context) error) error
	if use != nil {
		guarded = func(ctx context.Context, _ func(context.Context) error) error { return use(ctx) }
	}
	return e.withRecoveryExclusionGuardedLocked(ctx, observe, destinations, guarded)
}

func (e *Engine) withRecoveryExclusionGuardedLocked(ctx context.Context, observe, destinations func(context.Context) error, use func(context.Context, func(context.Context) error) error) (result error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if observe == nil || destinations == nil {
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
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = e.matches(record); err != nil {
			return ErrConflict
		}
	}
	for _, unit := range []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket"} {
		if err = ctx.Err(); err != nil {
			return err
		}
		data, err := e.readConfiguration(installed, "etc/systemd/system/"+unit)
		if err != nil || bytes.Count(data, []byte("ConditionPathExists=!/var/lib/homenode-install/recovery-blocked\n")) != 1 {
			return ErrConflict
		}
	}
	if err = destinations(ctx); err != nil {
		return err
	}
	if err = observe(ctx); err != nil {
		return err
	}
	if use != nil {
		currentInstallation, err := e.load()
		if err != nil || currentInstallation.ID != installed.ID || currentInstallation.Digest != installed.Digest || currentInstallation.Phase != "installed" {
			return ErrConflict
		}
		for _, record := range installed.Items {
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = e.matches(record); err != nil {
				return ErrConflict
			}
		}
		if err = destinations(ctx); err != nil {
			return err
		}
		// Recheck exclusion before granting the consumer access. Do not recreate
		// missing markers or release them after an interrupted consumer.
		if err = e.requireRecoveryActivationBlock(ctx); err != nil {
			return err
		}
		current, err := e.journalRoot.Lstat("recovery-blocked")
		if err != nil || !os.SameFile(markerBefore, current) {
			return ErrConflict
		}
		verifyRetained := func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			currentInstallation, err := e.load()
			if err != nil || currentInstallation.ID != installed.ID || currentInstallation.Digest != installed.Digest || currentInstallation.Phase != "installed" {
				return ErrConflict
			}
			for _, record := range installed.Items {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := e.matches(record); err != nil {
					return ErrConflict
				}
			}
			if err := destinations(ctx); err != nil {
				return err
			}
			if err := e.requireRecoveryActivationBlock(ctx); err != nil {
				return err
			}
			current, err := e.journalRoot.Lstat("recovery-blocked")
			if err != nil || !os.SameFile(markerBefore, current) {
				return ErrConflict
			}
			return ctx.Err()
		}
		retainedGuard := func(ctx context.Context) error {
			if err := verifyRetained(ctx); err != nil {
				return err
			}
			if err := observe(ctx); err != nil {
				return err
			}
			return verifyRetained(ctx)
		}
		if err = use(ctx, retainedGuard); err != nil {
			return err
		}
		if err = observe(ctx); err != nil {
			return err
		}
	}
	// Configuration may have changed while querying the manager. Retain the
	// original journal identity and reverify its owned bytes before success.
	currentInstallation, err := e.load()
	if err != nil || currentInstallation.ID != installed.ID || currentInstallation.Digest != installed.Digest || currentInstallation.Phase != "installed" {
		return ErrConflict
	}
	for _, record := range installed.Items {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = e.matches(record); err != nil {
			return ErrConflict
		}
	}
	if err = destinations(ctx); err != nil {
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
