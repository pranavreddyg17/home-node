//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/updates"
)

// StageUpdateInspection uses only the staging directory in the completed
// installer journal. The maintenance caller supplies its acquired signed
// release and protected operation ID. It activates no service and grants no
// installation authority; existing intent requires explicit recovery.
func (e *Engine) StageUpdateInspection(ctx context.Context, release *updates.AcquiredRelease, operation string) error {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	return e.stageUpdateInspectionOwned(ctx, release, operation)
}

func (e *Engine) stageUpdateInspectionOwned(ctx context.Context, release *updates.AcquiredRelease, operation string) (resultErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	configuration, err := e.readUpdateRepositoryLocked(ctx)
	if err != nil {
		return err
	}
	if release == nil || release.Metadata.Platform != "ubuntu-24.04-amd64" || release.Metadata.Sequence < configuration.MinimumSequence || release.Metadata.CatalogVersion < configuration.MinimumCatalogVersion {
		return ErrConflict
	}
	j, err := e.load()
	if err != nil {
		return err
	}
	found := false
	for _, item := range j.Items {
		if item.Path != "var/lib/homenode-update/inspection" {
			continue
		}
		if found || !item.Directory || item.Mode != 0700 || item.UID != e.owner || item.GID != 0 || item.State != "created" && item.State != "existing" {
			return ErrConflict
		}
		found = true
	}
	if !found {
		return ErrConflict
	}
	staging, err := e.host.OpenRoot("var/lib/homenode-update/inspection")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, staging.Close()) }()
	return updates.StageInspectionPackage(ctx, staging, release, operation)
}
