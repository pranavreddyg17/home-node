package install

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// Independent publisher trust is supplied by the caller, never read from the
// installed key alone. Caller retains installation exclusion around this read.
func (e *Engine) installedGuestStorageCatalog(ctx context.Context, publisher ed25519.PublicKey, minimum int64, now time.Time) (catalog.Manifest, uint32, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Manifest{}, 0, err
	}
	if len(publisher) != ed25519.PublicKeySize || minimum < 1 {
		return catalog.Manifest{}, 0, ErrPlan
	}
	installed, err := e.load()
	if err != nil {
		return catalog.Manifest{}, 0, err
	}
	if installed.Phase != "installed" {
		return catalog.Manifest{}, 0, ErrConflict
	}
	var sourceGID uint32
	for _, item := range installed.Items {
		if err := e.matches(item); err != nil {
			return catalog.Manifest{}, 0, ErrConflict
		}
		if item.Path == "var/lib/homenode/images" {
			if !item.Directory || item.UID != 0 || item.GID <= 0 || item.GID > 1<<31-1 || item.Mode != 0710 || sourceGID != 0 {
				return catalog.Manifest{}, 0, ErrConflict
			}
			sourceGID = uint32(item.GID)
		}
	}
	if sourceGID == 0 {
		return catalog.Manifest{}, 0, ErrConflict
	}
	key, err := e.readConfiguration(installed, "etc/homenode/catalog.pub")
	if err != nil || string(key) != hex.EncodeToString(publisher)+"\n" {
		return catalog.Manifest{}, 0, ErrConflict
	}
	floorBytes, err := e.readConfiguration(installed, "etc/homenode/catalog-floor")
	if err != nil {
		return catalog.Manifest{}, 0, err
	}
	floor, err := strconv.ParseInt(strings.TrimSuffix(string(floorBytes), "\n"), 10, 64)
	if err != nil || floor < minimum || string(floorBytes) != strconv.FormatInt(floor, 10)+"\n" {
		return catalog.Manifest{}, 0, ErrConflict
	}
	data, err := e.readConfiguration(installed, "var/lib/homenode/catalog/catalog.json")
	if err != nil {
		return catalog.Manifest{}, 0, err
	}
	manifest, err := catalog.Verify(data, map[string]ed25519.PublicKey{catalog.KeyID(publisher): publisher}, floor, now)
	if err != nil || len(manifest.Images) != 3 {
		return catalog.Manifest{}, 0, catalog.ErrUntrusted
	}
	placed, err := e.loadImageJournal()
	if err != nil || placed.ConfigurationID != installed.ID || placed.CatalogDigest != digest(data) || placed.Completed != len(manifest.Images) {
		return catalog.Manifest{}, 0, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return catalog.Manifest{}, 0, err
	}
	return manifest, sourceGID, nil
}
