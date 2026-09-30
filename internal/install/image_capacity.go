package install

import (
	"context"
	"crypto/ed25519"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// Called under the engine lock. Credit applies only to configuration-matching
// owned images, never arbitrary existing files or caller-provided capacities.
func (e *Engine) configurationImageCredit(ctx context.Context, c Configuration, now time.Time) (uint64, bool, error) {
	j, err := e.loadImageJournal()
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	config, err := e.load()
	if err != nil {
		return 0, false, err
	}
	if config.Phase != "installed" || j.ConfigurationID != config.ID || j.CatalogDigest != digest(c.Catalog) {
		return 0, false, ErrConflict
	}
	// Rebuild the exact expected content plan without using the disk admission
	// result. Only a matching existing transaction is eligible for image credit.
	candidate := c
	candidate.Capacity.FreeDiskBytes = math.MaxUint64
	preview, err := ConfigurationPlan(candidate, now)
	if err != nil {
		return 0, false, err
	}
	_, hash, err := planRecords(preview.Plan, e.owner)
	if err != nil || hash != config.Digest {
		return 0, false, ErrConflict
	}
	for _, r := range config.Items {
		if err = e.matches(r); err != nil {
			return 0, false, ErrConflict
		}
	}
	manifest, err := catalog.Verify(c.Catalog, map[string]ed25519.PublicKey{catalog.KeyID(c.Publisher): c.Publisher}, c.MinimumCatalogVersion, now)
	if err != nil || j.Completed > len(manifest.Images) {
		return 0, false, catalog.ErrUntrusted
	}
	images, err := protectedChildPath(e.host, "var/lib/homenode/images", e.owner, c.Accounts.QEMUGID)
	if err != nil {
		return 0, false, err
	}
	defer images.Close()
	var credit uint64
	cleaned := false
	seen := map[string]bool{}
	for index, image := range manifest.Images {
		if err = ctx.Err(); err != nil {
			return 0, cleaned, err
		}
		final := image.SHA256 + ".raw"
		if _, err = images.Lstat(final); err == nil {
			if err = verifyPlacedImage(ctx, images, final, image, e.owner, c.Accounts.QEMUGID); err != nil {
				return 0, cleaned, err
			}
			if !seen[final] {
				credit += uint64(image.Bytes)
				seen[final] = true
			}
		} else if !os.IsNotExist(err) {
			return 0, cleaned, err
		} else if index < j.Completed {
			return 0, cleaned, ErrConflict
		}
		stage := ".homenode-" + config.ID + "-image-" + strconv.Itoa(index) + ".stage"
		if _, err = images.Lstat(stage); err == nil {
			if err = removeImageStage(images, stage, e.owner); err != nil {
				return 0, cleaned, err
			}
			cleaned = true
		} else if !os.IsNotExist(err) {
			return 0, cleaned, err
		}
	}
	if cleaned {
		if err = syncDirectory(images, "."); err != nil {
			return 0, cleaned, err
		}
	}
	return credit, cleaned, nil
}
