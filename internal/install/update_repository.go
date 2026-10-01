package install

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/updates"
)

// ReadUpdateRepository accepts acquisition policy only from the completed
// installer ownership journal. It grants no update or installation authority.
func (e *Engine) ReadUpdateRepository(ctx context.Context) (updates.RepositoryConfiguration, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return updates.RepositoryConfiguration{}, ErrConflict
	}
	return e.readUpdateRepositoryOwned(ctx)
}

func (e *Engine) readUpdateRepositoryOwned(ctx context.Context) (updates.RepositoryConfiguration, error) {
	var zero updates.RepositoryConfiguration
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	j, err := e.load()
	if err != nil {
		return zero, err
	}
	if _, err = updateBootstrapRecord(j, e.owner); err != nil {
		return zero, err
	}
	var configuration *record
	for _, item := range j.Items {
		if err = e.matches(item); err != nil {
			return zero, err
		}
		if item.Path == "etc/homenode/update-repository.json" {
			if configuration != nil || item.Directory || item.Mode != 0400 || item.UID != e.owner || item.GID != 0 || item.State != "created" || len(item.SHA256) != 64 {
				return zero, ErrConflict
			}
			copy := item
			configuration = &copy
		}
	}
	if configuration == nil {
		return zero, ErrConflict
	}
	file, err := e.host.OpenFile(configuration.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return zero, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 8193))
	if err = errors.Join(readErr, file.Close(), ctx.Err()); err != nil {
		return zero, err
	}
	if len(data) > 8192 || digest(data) != configuration.SHA256 {
		return zero, ErrConflict
	}
	return updates.ParseRepositoryConfiguration(data)
}
