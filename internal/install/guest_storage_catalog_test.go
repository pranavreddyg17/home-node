package install

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

func TestRootGuestStorageCatalogRequiresReleaseTrustAndPlacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	e, c, _, _, source, now := imagePlacementFixture(t)
	defer e.Close()
	ctx := context.Background()
	if _, _, err := e.installedGuestStorageCatalog(ctx, c.Publisher, c.MinimumCatalogVersion, now); !errors.Is(err, ErrConflict) {
		t.Fatal("unplaced catalog admitted", err)
	}
	if err := e.placeImages(ctx, source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
		t.Fatal(err)
	}
	manifest, gid, err := e.installedGuestStorageCatalog(ctx, c.Publisher, c.MinimumCatalogVersion, now)
	if err != nil || len(manifest.Images) != 3 || gid != uint32(c.Accounts.QEMUGID) {
		t.Fatal("placed trusted catalog refused", gid, err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	observations := 0
	if result, group, err := e.guestStorageCatalogForJournal(ctx, installed, c.Publisher, c.MinimumCatalogVersion, now, func(context.Context) error {
		observations++
		if observations == 2 {
			return ErrConflict
		}
		return nil
	}); !errors.Is(err, ErrConflict) || group != 0 || len(result.Images) != 0 {
		t.Fatal("revoked installation guard returned catalog authority", group, err)
	}
	wrong := append(ed25519.PublicKey(nil), c.Publisher...)
	wrong[0] ^= 1
	if _, _, err := e.installedGuestStorageCatalog(ctx, wrong, c.MinimumCatalogVersion, now); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign release trust admitted", err)
	}
	if _, _, err := e.installedGuestStorageCatalog(ctx, c.Publisher, c.MinimumCatalogVersion+1, now); !errors.Is(err, ErrConflict) {
		t.Fatal("downgraded installed floor admitted", err)
	}
	if _, _, err := e.installedGuestStorageCatalog(ctx, c.Publisher, c.MinimumCatalogVersion, now.Add(2*time.Hour)); !errors.Is(err, catalog.ErrUntrusted) {
		t.Fatal("expired catalog admitted", err)
	}
	placed, err := e.loadImageJournal()
	if err != nil {
		t.Fatal(err)
	}
	placed.Completed--
	if err := e.saveImageJournal(placed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.installedGuestStorageCatalog(ctx, c.Publisher, c.MinimumCatalogVersion, now); !errors.Is(err, ErrConflict) {
		t.Fatal("incomplete placement admitted", err)
	}
}

func TestGuestStorageCatalogRequiresTransitionGuard(t *testing.T) {
	e := &Engine{}
	if _, _, err := e.guestStorageCatalogForJournal(context.Background(), journal{Phase: "installed"}, make(ed25519.PublicKey, ed25519.PublicKeySize), 1, time.Now(), nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing transition guard admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := e.guestStorageCatalogForJournal(ctx, journal{}, nil, 0, time.Now(), nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled transition catalog admitted", err)
	}
}
