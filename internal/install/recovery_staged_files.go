package install

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// withRecoveryStagedFiles lends verified read-only copies while retaining the
// installer lock. Consumers must not close or retain descriptors, reenter the
// engine, or infer runtime/publication authority from this data-only scope.
func (e *Engine) withRecoveryStagedFiles(ctx context.Context, expected recoveryIntent, use func(context.Context, []backup.PreparedRecoveryFile) error) (result error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	if !e.mu.TryLock() {
		return ErrConflict
	}
	defer e.mu.Unlock()
	return e.withRecoveryStagedFilesLocked(ctx, expected, use)
}

// withRecoveryStagedFilesLocked composes source qualification into an operation
// that already retains e.mu and the engine's cross-process installation lock.
// It does not acquire a second lock or grant runtime/publication authority.
func (e *Engine) withRecoveryStagedFilesLocked(ctx context.Context, expected recoveryIntent, use func(context.Context, []backup.PreparedRecoveryFile) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	if err := e.reconcileRecoveryStaging(ctx, expected); err != nil {
		return err
	}
	files := []backup.PreparedRecoveryFile{{Name: "management.db", Bytes: expected.Recovery.ManagementBytes, SHA256: expected.Recovery.ManagementSHA256}}
	stages := []string{".recovery-management.copy"}
	for _, disk := range expected.Recovery.Disks {
		files = append(files, backup.PreparedRecoveryFile{Name: disk.InstanceID + ".raw", Bytes: disk.Bytes, SHA256: disk.SourceSHA256})
		stages = append(stages, ".recovery-"+disk.InstanceID+".copy")
	}
	defer func() {
		for _, file := range files {
			if file.File != nil {
				result = errors.Join(result, file.File.Close())
			}
		}
	}()
	for i, stage := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := e.journalRoot.Lstat(stage)
		if err != nil {
			return err
		}
		file, err := e.journalRoot.OpenFile(stage, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		files[i].File = file
		opened, err := file.Stat()
		if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || !owned(opened, e.owner) || opened.Size() != files[i].Bytes {
			return ErrConflict
		}
	}
	// Reconcile after opening to bind the borrowed descriptors to current copies.
	if err := e.reconcileRecoveryStaging(ctx, expected); err != nil {
		return err
	}
	for i, stage := range stages {
		current, err := e.journalRoot.Lstat(stage)
		opened, statErr := files[i].File.Stat()
		if err != nil || statErr != nil || !os.SameFile(current, opened) {
			return ErrConflict
		}
	}
	// Lend a separate inventory slice so consumer metadata edits cannot replace
	// the descriptor handles retained for verification and cleanup.
	if err := use(ctx, append([]backup.PreparedRecoveryFile(nil), files...)); err != nil {
		return err
	}
	if err := e.reconcileRecoveryStaging(ctx, expected); err != nil {
		return err
	}
	for i, stage := range stages {
		current, err := e.journalRoot.Lstat(stage)
		opened, statErr := files[i].File.Stat()
		if err != nil || statErr != nil || !os.SameFile(current, opened) {
			return ErrConflict
		}
	}
	return ctx.Err()
}
