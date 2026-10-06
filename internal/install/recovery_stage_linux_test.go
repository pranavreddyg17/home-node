//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRootRecoveryManagementStagingReplay(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	e, c, host, journal, _, now := imagePlacementFixtureWithMaintenance(t, &MaintenanceAccount{UID: 803, GID: 803})
	defer func() { e.Close() }()
	store, err := state.Open(filepath.Join(t.TempDir(), "live"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed) VALUES(1,?,1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	sourcePath := t.TempDir()
	if err = os.Chmod(sourcePath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecoverySnapshot(context.Background(), sourcePath); err != nil {
		t.Fatal(err)
	}
	source, err := os.OpenRoot(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	data, err := source.ReadFile("snapshot.db")
	if err != nil {
		t.Fatal(err)
	}
	manifest := backup.Manifest{Version: 1, CreatedAt: now, Release: "0.1.0~dev", Platform: "ubuntu-24.04-amd64", ManagementSchema: 4, CatalogVersion: 4, Files: []backup.BackupFile{{Workload: "management", Name: "snapshot.db", Bytes: int64(len(data)), SHA256: digest(data), DataSchema: 4}}}
	preparedPath := t.TempDir()
	if err = os.Chmod(preparedPath, 0700); err != nil {
		t.Fatal(err)
	}
	prepared, err := os.OpenRoot(preparedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err = backup.PreparePrivateRecovery(context.Background(), source, prepared, strings.Repeat("a", 64), manifest, backup.RestorePolicy{MinimumCatalogVersion: 4}); err != nil {
		t.Fatal(err)
	}
	if err = filepath.WalkDir(preparedPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Chown(path, 803, 803)
	}); err != nil {
		t.Fatal(err)
	}
	lease, err := backup.OpenPreparedRecovery(context.Background(), prepared, 803)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	interrupted := errors.New("copy acknowledgement interrupted")
	e.checkpoint = func(stage, name string) error {
		if stage == "recovery-copy" {
			return interrupted
		}
		return nil
	}
	if _, err = e.stageRecoveryCopies(context.Background(), lease, manifest, c, now); !errors.Is(err, interrupted) {
		t.Fatal("copy boundary", err)
	}
	if _, err = e.journalRoot.Lstat("recovery-staged.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("premature completion", err)
	}
	before, err := e.journalRoot.Lstat(".recovery-management.copy")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, host, journal)
	preview, err := e.stageRecoveryCopies(context.Background(), lease, manifest, c, now)
	if err != nil || preview.Recovery.OwnerID == "" || len(preview.Recovery.Disks) != 0 {
		t.Fatal("management staging replay", preview, err)
	}
	after, err := e.journalRoot.Lstat(".recovery-management.copy")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("completed copy replaced", err)
	}
	if _, err = e.journalRoot.Lstat("recovery-staged.json"); err != nil {
		t.Fatal("completion receipt missing", err)
	}
}
