//go:build linux || darwin

package install

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

func TestRecoveryConfigurationRequiresReplacementTrustAndDiskSize(t *testing.T) {
	c, catalogManifest, _, now := configurationFixture(t)
	c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.WriteFile("maintenance.lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := backup.OpenPreparedRecovery(context.Background(), root, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	source := backup.Manifest{Files: []backup.BackupFile{{Workload: "files", Bytes: catalogManifest.Images[0].DataBytes - 1}}}
	if preview, err := RecoveryConfigurationPlan(context.Background(), lease, source, c, now); !errors.Is(err, ErrPlan) || preview.Recovery.OwnerID != "" {
		t.Fatal("incompatible disk size admitted", preview, err)
	}
	invalid := c
	invalid.MinimumCatalogVersion = catalogManifest.Version + 1
	if _, err := RecoveryConfigurationPlan(context.Background(), lease, source, invalid, now); err == nil {
		t.Fatal("replacement catalog floor bypassed")
	}
	source.Files[0].Bytes++
	if _, err := RecoveryConfigurationPlan(context.Background(), lease, source, c, now); !errors.Is(err, backup.ErrManifest) {
		t.Fatal("foreign lease owner or unprepared data admitted", err)
	}
	if _, err := lease.InventoryForOwner(context.Background(), uint32(os.Geteuid())+1, source, backup.RestorePolicy{}); !errors.Is(err, backup.ErrManifest) {
		t.Fatal("lease ownership binding bypassed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RecoveryConfigurationPlan(ctx, lease, source, c, now); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation bypassed", err)
	}
}
