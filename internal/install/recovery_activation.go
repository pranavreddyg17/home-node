package install

import (
	"context"
	"errors"
	"os"
	"syscall"
)

// blockRecoveryActivation retains a persistent fail-closed marker used by the
// installed service/socket conditions. It does not stop already-running units
// or prove manager reload/guest emptiness; publication must check those too.
// No automatic release is provided before bootstrap/restore acceptance exists.
func (e *Engine) blockRecoveryActivation(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := e.journalRoot.OpenFile("recovery-blocked", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
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
