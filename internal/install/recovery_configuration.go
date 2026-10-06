package install

import (
	"context"
	"crypto/ed25519"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// RecoveryConfigurationPreview joins disconnected recovery qualification with
// the replacement host's independently trusted configuration. It is not an
// ownership journal or permission to activate services. The caller retains the
// prepared lease through subsequent journaled data handoff.
type RecoveryConfigurationPreview struct {
	Configuration ConfigurationPreview
	Recovery      backup.PreparedRecoveryInventory
}

func RecoveryConfigurationPlan(ctx context.Context, prepared *backup.PreparedRecoveryLease, source backup.Manifest, c Configuration, now time.Time) (RecoveryConfigurationPreview, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	configuration, err := ConfigurationPlan(c, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if c.Maintenance == nil || prepared == nil {
		return RecoveryConfigurationPreview{}, ErrPlan
	}
	manifest, err := catalog.Verify(c.Catalog, map[string]ed25519.PublicKey{catalog.KeyID(c.Publisher): c.Publisher}, c.MinimumCatalogVersion, now)
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	approved := make(map[string]string, 2)
	for _, workload := range []string{"files", "ai"} {
		image, err := manifest.Image(workload)
		if err != nil {
			return RecoveryConfigurationPreview{}, err
		}
		approved[workload] = image.SHA256
	}
	// Reject incompatible declared sizes before expensive filesystem inspection.
	for _, file := range source.Files {
		if file.Workload == "management" {
			continue
		}
		image, err := manifest.Image(file.Workload)
		if err != nil || file.Bytes != image.DataBytes {
			return RecoveryConfigurationPreview{}, ErrPlan
		}
	}
	inventory, err := prepared.InventoryForOwner(ctx, c.Maintenance.UID, source, backup.RestorePolicy{MinimumCatalogVersion: c.MinimumCatalogVersion, ApprovedImages: approved})
	if err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	if err = ctx.Err(); err != nil {
		return RecoveryConfigurationPreview{}, err
	}
	configuration.Pending = append(configuration.Pending, "journal and copy recovered data with installed ownership", "reconstruct stopped runtime policy with fresh UID leases", "bootstrap new owner and reenroll devices", "verify restored workloads before activation")
	return RecoveryConfigurationPreview{Configuration: configuration, Recovery: inventory}, nil
}
