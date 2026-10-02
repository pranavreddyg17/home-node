//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/updates"
	servicetemplates "github.com/pranavreddyg17/home-node/packaging/systemd"
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
	if err := e.verifyUpdateReleaseEvidenceLocked(ctx, release); err != nil {
		return err
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
	if err := e.verifyUpdateReleaseEvidenceLocked(ctx, release); err != nil {
		return nil, err
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

// ReadRecordedUpdateInspectionResult readmits the completed installer's owned
// package/service policy before reading retained result evidence. release and
// execution must come from protected signed acquisition/operation state. This
// grants no installation authority and requires the caller's prior stage closed.
func (e *Engine) ReadRecordedUpdateInspectionResult(ctx context.Context, release *updates.AcquiredRelease, operation string, execution updates.InspectionExecution) (updates.InspectionResult, error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return updates.InspectionResult{}, ErrConflict
	}
	return e.readRecordedUpdateInspectionResultOwned(ctx, release, operation, execution)
}

// CollectAndPublishUpdateInspectionResult readmits installer-owned release and
// service policy before collecting completion and local-journal evidence itself.
// The caller must close its prior stage and retain execution independently. This
// neither starts/stops the worker nor grants installation authority.
func (e *Engine) CollectAndPublishUpdateInspectionResult(ctx context.Context, release *updates.AcquiredRelease, operation string, execution updates.InspectionExecution) (updates.InspectionResult, error) {
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return updates.InspectionResult{}, ErrConflict
	}
	return e.collectAndPublishUpdateInspectionResultOwned(ctx, release, operation, execution)
}

func (e *Engine) collectAndPublishUpdateInspectionResultOwned(ctx context.Context, release *updates.AcquiredRelease, operation string, execution updates.InspectionExecution) (updates.InspectionResult, error) {
	return e.withUpdateInspectionResultOwned(ctx, release, operation, execution, (*updates.InspectionStage).CollectAndPublishInspectionResult)
}

func (e *Engine) readRecordedUpdateInspectionResultOwned(ctx context.Context, release *updates.AcquiredRelease, operation string, execution updates.InspectionExecution) (updates.InspectionResult, error) {
	return e.withUpdateInspectionResultOwned(ctx, release, operation, execution, (*updates.InspectionStage).ReadRecordedInspectionResult)
}

func (e *Engine) withUpdateInspectionResultOwned(ctx context.Context, release *updates.AcquiredRelease, operation string, execution updates.InspectionExecution, collect func(*updates.InspectionStage, context.Context, *os.Root, updates.InspectionExecution) (updates.InspectionResult, error)) (updates.InspectionResult, error) {
	var zero updates.InspectionResult
	e.mu.Lock()
	defer e.mu.Unlock()
	stage, err := e.openUpdateInspectionLocked(ctx, release, operation)
	if err != nil {
		return zero, err
	}
	if err := e.requireInspectionServiceLocked(); err != nil {
		return zero, errors.Join(err, stage.Close())
	}
	parent, err := e.host.OpenRoot("var/lib/homenode-update")
	if err != nil {
		return zero, errors.Join(err, stage.Close())
	}
	metadata, packageHash, packageLength := release.Metadata, release.PackageSHA256, release.PackageLength
	result, readErr := collect(stage, ctx, parent, execution)
	// Collection may wait on manager/journal processes. Recheck installer policy
	// before exposing evidence rather than relying only on its earlier admission.
	if readErr == nil {
		configuration, policyErr := e.readUpdateRepositoryLocked(ctx)
		readErr = policyErr
		if readErr == nil && (release.Metadata != metadata || release.PackageSHA256 != packageHash || release.PackageLength != packageLength) {
			readErr = ErrConflict
		}
		if readErr == nil && (release.Metadata.Sequence < configuration.MinimumSequence || release.Metadata.CatalogVersion < configuration.MinimumCatalogVersion) {
			readErr = ErrConflict
		}
		if readErr == nil {
			readErr = e.verifyUpdateReleaseEvidenceLocked(ctx, release)
		}
		if readErr == nil {
			readErr = e.requireInspectionServiceLocked()
		}
	}
	closeErr := errors.Join(parent.Close(), stage.Close())
	if readErr != nil || closeErr != nil {
		return zero, errors.Join(readErr, closeErr)
	}
	return result, nil
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
	if err := e.requireInspectionServiceLocked(); err != nil {
		return nil, errors.Join(err, stage.Close())
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

func (e *Engine) requireInspectionServiceLocked() error {
	// Filesystem refusal supplements, but cannot replace, checking the manager's
	// effective unit and DropInPaths immediately before activation.
	for _, base := range []string{"etc/systemd/system", "run/systemd/system", "usr/local/lib/systemd/system", "usr/lib/systemd/system", "lib/systemd/system"} {
		for _, name := range []string{"service.d", "homenode-.service.d", "homenode-inspect.service.d"} {
			if _, err := e.host.Lstat(base + "/" + name); !errors.Is(err, os.ErrNotExist) {
				return errors.Join(ErrConflict, err)
			}
		}
	}

	reviewed, err := servicetemplates.Unit("homenode-inspect.service")
	if err != nil {
		return err
	}
	journal, err := e.load()
	if err != nil {
		return err
	}
	found := false
	for _, item := range journal.Items {
		if item.Path != "etc/systemd/system/homenode-inspect.service" {
			continue
		}
		if found || item.Directory || item.Mode != 0644 || item.UID != e.owner || item.GID != 0 || item.SHA256 != digest(reviewed) || item.State != "created" && item.State != "existing" {
			return ErrConflict
		}
		if err := e.matches(item); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return ErrConflict
	}
	return nil
}
