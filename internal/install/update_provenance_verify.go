package install

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/updates"
)

// VerifyUpdateReleaseProvenance verifies acquired package/provenance using owned
// trust policy. SBOM/vulnerability/owner approval/install gates remain required.
func (e *Engine) VerifyUpdateReleaseProvenance(ctx context.Context, release *updates.AcquiredRelease) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	return e.verifyUpdateReleaseProvenanceOwned(ctx, release)
}

func (e *Engine) verifyUpdateReleaseProvenanceOwned(ctx context.Context, release *updates.AcquiredRelease) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.verifyUpdateReleaseProvenanceLocked(ctx, release)
}

// Caller holds e.mu across acquisition and qualification.
func (e *Engine) verifyUpdateReleaseProvenanceLocked(ctx context.Context, release *updates.AcquiredRelease) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if release == nil || release.Package == nil {
		return ErrConflict
	}
	configuration, err := e.readUpdateRepositoryLocked(ctx)
	if err != nil {
		return err
	}
	if release.Metadata.Platform != "ubuntu-24.04-amd64" || release.Metadata.Sequence < configuration.MinimumSequence || release.Metadata.CatalogVersion < configuration.MinimumCatalogVersion {
		return ErrConflict
	}
	policy, err := e.readUpdateProvenancePolicyLocked(ctx)
	if err != nil {
		return err
	}
	metadata, hash, length := release.Metadata, release.PackageSHA256, release.PackageLength
	actual, count, err := updates.PackageIdentity(ctx, release.Package)
	if err != nil {
		return err
	}
	if actual != hash || count != length {
		return ErrConflict
	}
	if err := updates.VerifyReleaseProvenance(release.Provenance, hash, policy); err != nil {
		return err
	}
	actual, count, err = updates.PackageIdentity(ctx, release.Package)
	if err != nil {
		return err
	}
	if actual != hash || count != length || release.Metadata != metadata || release.PackageSHA256 != hash || release.PackageLength != length {
		return ErrConflict
	}
	current, err := e.readUpdateProvenancePolicyLocked(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(policy, current) {
		return ErrConflict
	}
	return ctx.Err()
}
