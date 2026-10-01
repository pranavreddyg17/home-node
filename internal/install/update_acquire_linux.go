//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/updates"
)

// AcquireUpdateRelease uses only installer-owned trust, repository scopes and
// staging directories. currentSchema must be measured from protected host state
// by the maintenance caller. The result still requires evidence qualification,
// fresh owner approval and the journaled installation lifecycle.
func (e *Engine) AcquireUpdateRelease(ctx context.Context, target string, currentSchema int) (result *updates.AcquiredRelease, resultErr error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return nil, ErrConflict
	}
	return e.acquireUpdateReleaseOwned(ctx, target, currentSchema)
}

func (e *Engine) acquireUpdateReleaseOwned(ctx context.Context, target string, currentSchema int) (result *updates.AcquiredRelease, resultErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	configuration, err := e.readUpdateRepositoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := configuration.Policy(currentSchema)
	if err != nil {
		return nil, err
	}
	// Only this completed ownership journal may bootstrap or resume the cache.
	// Completed initialization preserves the current rotated root and floors.
	if err = e.initializeUpdateCacheLocked(ctx); err != nil {
		return nil, err
	}
	provisioned, err := e.host.OpenRoot("var/lib/homenode-update")
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, provisioned.Close())
		if resultErr != nil && result != nil {
			resultErr = errors.Join(resultErr, result.Close())
			result = nil
		}
	}()
	staging, err := provisioned.OpenRoot("downloads")
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, staging.Close(), ctx.Err()) }()
	return updates.AcquireRelease(ctx, provisioned, staging, configuration.MetadataURL, configuration.TargetsURL, target, policy)
}
