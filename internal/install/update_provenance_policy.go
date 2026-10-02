package install

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/updates"
	"io"
	"os"
	"runtime"
	"syscall"
)

// ReadUpdateProvenancePolicy accepts pinned trust only from completed installer
// ownership. It grants no release approval or installation authority.
func (e *Engine) ReadUpdateProvenancePolicy(ctx context.Context) (updates.ProvenancePolicy, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return updates.ProvenancePolicy{}, ErrConflict
	}
	return e.readUpdateProvenancePolicyOwned(ctx)
}

func (e *Engine) readUpdateProvenancePolicyOwned(ctx context.Context) (updates.ProvenancePolicy, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.readUpdateProvenancePolicyLocked(ctx)
}

func (e *Engine) readUpdateProvenancePolicyLocked(ctx context.Context) (updates.ProvenancePolicy, error) {
	var zero updates.ProvenancePolicy
	if _, err := e.readUpdateRepositoryLocked(ctx); err != nil {
		return zero, err
	}
	journal, err := e.load()
	if err != nil {
		return zero, err
	}
	var policy *record
	for _, item := range journal.Items {
		if item.Path != "etc/homenode/update-provenance.json" {
			continue
		}
		if policy != nil || item.Directory || item.Mode != 0400 || item.UID != e.owner || item.GID != 0 || item.State != "created" {
			return zero, ErrConflict
		}
		copy := item
		policy = &copy
	}
	if policy == nil {
		return zero, ErrConflict
	}
	file, err := e.host.OpenFile(policy.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return zero, err
	}
	check := func() error {
		info, err := file.Stat()
		if err != nil {
			return err
		}
		native, ok := info.Sys().(*syscall.Stat_t)
		if !ok || native.Mode != syscall.S_IFREG|0400 || int(native.Uid) != e.owner || native.Gid != 0 || native.Nlink != 1 {
			return ErrConflict
		}
		return nil
	}
	if err := check(); err != nil {
		return zero, errors.Join(err, file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16385))
	err = errors.Join(readErr, check(), file.Close(), ctx.Err())
	if err != nil {
		return zero, err
	}
	if len(data) > 16384 || digest(data) != policy.SHA256 {
		return zero, ErrConflict
	}
	return updates.ParseProvenancePolicy(data)
}
