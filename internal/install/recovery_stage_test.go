package install

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestRecoveryStagingRefusesWithoutInstalledConfiguration(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	c, _, _, now := configurationFixture(t)
	if result, err := e.stageRecoveryCopies(context.Background(), nil, backup.Manifest{}, c, now); !errors.Is(err, ErrConflict) || result.Recovery.OwnerID != "" {
		t.Fatal("uninstalled host admitted recovery", result, err)
	}
	for _, name := range []string{"recovery.json", ".recovery-management.copy"} {
		if _, err := e.journalRoot.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("refused recovery wrote data", name, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.stageRecoveryCopies(ctx, nil, backup.Manifest{}, c, now); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
}

func TestRootRecoveryStagingRefusesDifferentInstalledConfiguration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	e, c, _, _, _, now := imagePlacementFixtureWithMaintenance(t, &MaintenanceAccount{UID: 803, GID: 803})
	defer e.Close()
	c.Policy.Generation++
	if _, err := e.stageRecoveryCopies(context.Background(), nil, backup.Manifest{}, c, now); !errors.Is(err, ErrConflict) {
		t.Fatal("different installed policy admitted", err)
	}
	if _, err := e.journalRoot.Lstat("recovery.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("conflicting configuration journaled", err)
	}
}
