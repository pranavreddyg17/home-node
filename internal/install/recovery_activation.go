package install

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

// blockRecoveryActivation retains a persistent fail-closed marker used by the
// installed service/socket conditions. It does not stop already-running units
// or prove manager reload/guest emptiness; publication must check those too.
// No automatic release is provided before bootstrap/restore acceptance exists.
func (e *Engine) blockRecoveryActivation(ctx context.Context) error {
	return e.recoveryActivationMarker(ctx, true)
}

func (e *Engine) requireRecoveryActivationBlock(ctx context.Context) error {
	return e.recoveryActivationMarker(ctx, false)
}

func (e *Engine) recoveryActivationMarker(ctx context.Context, create bool) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	flags := os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags = os.O_CREATE | os.O_EXCL | os.O_WRONLY | syscall.O_NOFOLLOW
	}
	file, err := e.journalRoot.OpenFile("recovery-blocked", flags, 0600)
	if errors.Is(err, os.ErrExist) {
		file, err = e.journalRoot.OpenFile("recovery-blocked", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != 0600 || info.Size() != 0 {
		return ErrConflict
	}
	var native unix.Stat_t
	if unix.Fstat(int(file.Fd()), &native) != nil || native.Nlink != 1 {
		return ErrConflict
	}
	current, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(info, current) {
		return ErrConflict
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = syncDirectory(e.journalRoot, "."); err != nil {
		return err
	}
	return ctx.Err()
}
