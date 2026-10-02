//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/updates"
)

// AcquireUpdateRelease uses only installer-owned trust, repository scopes and
// staging directories. The maintenance caller supplies its protected live state
// store; schema is observed rather than accepted as an integer argument.
// Acquisition enforces owned provenance policy before exposing package bytes.
// The result still requires SBOM/vulnerability evidence qualification,
// fresh owner approval and the journaled installation lifecycle.
func (e *Engine) AcquireUpdateRelease(ctx context.Context, target string, store *state.Store) (result *updates.AcquiredRelease, resultErr error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return nil, ErrConflict
	}
	return acquireWithObservedSchema(ctx, store, func(schema int) (*updates.AcquiredRelease, error) {
		return e.acquireUpdateReleaseOwned(ctx, target, schema)
	})
}

func (e *Engine) acquireUpdateReleaseOwned(ctx context.Context, target string, currentSchema int) (result *updates.AcquiredRelease, resultErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	configuration, err := e.readUpdateRepositoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	// Refuse missing or altered signer policy before network acquisition.
	if _, err := e.readUpdateProvenancePolicyLocked(ctx); err != nil {
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
	release, err := updates.AcquireRelease(ctx, provisioned, staging, configuration.MetadataURL, configuration.TargetsURL, target, policy)
	if err != nil {
		return nil, err
	}
	if err := e.verifyUpdateReleaseProvenanceLocked(ctx, release); err != nil {
		return nil, errors.Join(err, release.Close())
	}
	return release, nil
}
