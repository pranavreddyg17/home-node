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
	staging, err := e.openInspectionDirectoryLocked()
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, staging.Close()) }()
	return updates.StageInspectionPackage(ctx, staging, release, operation)
}

func (e *Engine) openInspectionDirectoryLocked() (*os.Root, error) {
	j, err := e.load()
	if err != nil {
		return nil, err
	}
	found := false
	for _, item := range j.Items {
		if item.Path != "var/lib/homenode-update/inspection" {
			continue
		}
		if found || !item.Directory || item.Mode != 0700 || item.UID != e.owner || item.GID != 0 || item.State != "created" && item.State != "existing" {
			return nil, ErrConflict
		}
		found = true
	}
	if !found {
		return nil, ErrConflict
	}
	return e.host.OpenRoot("var/lib/homenode-update/inspection")
}

// OpenUpdateInspection admits only installer-owned publication matching the
// caller's retained signed release and operation. The returned stage holds its
// execution lock until the caller closes it after worker/result verification.
func (e *Engine) OpenUpdateInspection(ctx context.Context, release *updates.AcquiredRelease, operation string) (*updates.InspectionStage, error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return nil, ErrConflict
	}
	return e.openUpdateInspectionOwned(ctx, release, operation)
}

func (e *Engine) openUpdateInspectionOwned(ctx context.Context, release *updates.AcquiredRelease, operation string) (*updates.InspectionStage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.openUpdateInspectionLocked(ctx, release, operation)
}

func (e *Engine) openUpdateInspectionLocked(ctx context.Context, release *updates.AcquiredRelease, operation string) (*updates.InspectionStage, error) {
	configuration, err := e.readUpdateRepositoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil || release.Metadata.Platform != "ubuntu-24.04-amd64" || release.Metadata.Sequence < configuration.MinimumSequence || release.Metadata.CatalogVersion < configuration.MinimumCatalogVersion {
		return nil, ErrConflict
	}
	staging, err := e.openInspectionDirectoryLocked()
	if err != nil {
		return nil, err
	}
	identity := updates.InspectionIdentity{OperationID: operation, Release: release.Metadata.Release, PackageSHA256: release.PackageSHA256, PackageLength: release.PackageLength}
	stage, err := updates.OpenInspectionStage(ctx, staging, identity)
	closeErr := staging.Close()
	if err != nil || closeErr != nil {
		if stage != nil {
			err = errors.Join(err, stage.Close())
		}
		return nil, errors.Join(err, closeErr)
	}
	return stage, nil
}

// PrepareUpdateInspectionLaunch admits the owned package and durably publishes
// fixed service inputs while retaining the exclusive execution lock. It does
// not activate a service or authorize installation. Existing launch state must
// be reconciled explicitly instead of being overwritten on retry.
func (e *Engine) PrepareUpdateInspectionLaunch(ctx context.Context, release *updates.AcquiredRelease, operation string) (*updates.InspectionStage, error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return nil, ErrConflict
	}
	return e.prepareUpdateInspectionLaunchOwned(ctx, release, operation)
}

func (e *Engine) prepareUpdateInspectionLaunchOwned(ctx context.Context, release *updates.AcquiredRelease, operation string) (*updates.InspectionStage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	stage, err := e.openUpdateInspectionLocked(ctx, release, operation)
	if err != nil {
		return nil, err
	}
	parent, err := e.host.OpenRoot("var/lib/homenode-update")
	if err != nil {
		return nil, errors.Join(err, stage.Close())
	}
	publishErr := stage.PublishEnvironment(ctx, parent)
	if publishErr == nil {
		publishErr = stage.VerifyEnvironment(ctx, parent)
	}
	closeErr := parent.Close()
	if publishErr != nil || closeErr != nil {
		return nil, errors.Join(publishErr, closeErr, stage.Close())
	}
	return stage, nil
}
