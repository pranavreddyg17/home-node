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

// InitializeUpdateCache accepts only the bootstrap and private directories
// owned by this installer's completed configuration journal. It never takes a
// browser-supplied root, changes the bootstrap pin or activates an updater.
func (e *Engine) InitializeUpdateCache(ctx context.Context) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	return e.initializeUpdateCacheOwned(ctx)
}

func updateBootstrapRecord(j journal, owner int) (record, error) {
	if j.Phase != "installed" {
		return record{}, ErrConflict
	}
	required := map[string]bool{"etc/homenode/update-root.json": false, "var/lib/homenode-update": false, "var/lib/homenode-update/metadata": false, "var/lib/homenode-update/downloads": false}
	var bootstrap record
	for _, item := range j.Items {
		seen, ok := required[item.Path]
		if !ok {
			continue
		}
		if seen || item.UID != owner || item.GID != 0 || item.State != "created" && item.State != "existing" {
			return record{}, ErrConflict
		}
		if item.Path == "etc/homenode/update-root.json" {
			if item.Directory || item.Mode != 0400 || len(item.SHA256) != 64 || item.State != "created" {
				return record{}, ErrConflict
			}
			bootstrap = item
		} else if !item.Directory || item.Mode != 0700 {
			return record{}, ErrConflict
		}
		required[item.Path] = true
	}
	for _, present := range required {
		if !present {
			return record{}, ErrConflict
		}
	}
	return bootstrap, nil
}

func (e *Engine) initializeUpdateCacheOwned(ctx context.Context) (resultErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initializeUpdateCacheLocked(ctx)
}

func (e *Engine) initializeUpdateCacheLocked(ctx context.Context) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	j, err := e.load()
	if err != nil {
		return err
	}
	bootstrap, err := updateBootstrapRecord(j, e.owner)
	if err != nil {
		return err
	}
	for _, item := range j.Items {
		if err = e.matches(item); err != nil {
			return err
		}
	}
	file, err := e.host.OpenFile(bootstrap.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	if len(data) > maxFileBytes || digest(data) != bootstrap.SHA256 {
		return ErrConflict
	}
	provisioned, err := e.host.OpenRoot("var/lib/homenode-update")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, provisioned.Close()) }()
	return updates.InitializeCacheRoot(ctx, provisioned, updates.BootstrapRoot{Data: data, SHA256: bootstrap.SHA256})
}
