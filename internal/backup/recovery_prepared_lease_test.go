//go:build linux || darwin

package backup

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestPreparedRecoveryLeasePinsExistingOwnedRoot(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	uid := uint32(os.Geteuid())
	if lease, err := OpenPreparedRecovery(ctx, root, uid); err == nil || lease != nil {
		t.Fatal("installer created missing preparation lock", err)
	}
	if _, err := root.Lstat("maintenance.lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection mutated unprepared directory", err)
	}
	writer, err := lockPrivateRunnerRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := OpenPreparedRecovery(ctx, root, uid); err == nil || lease != nil {
		t.Fatal("inspection overlapped preparation", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if lease, err := OpenPreparedRecovery(ctx, root, uid+1); err == nil || lease != nil {
		t.Fatal("inspection accepted unexpected owner", err)
	}
	lease, err := OpenPreparedRecovery(ctx, root, uid)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = root.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = lease.root.Stat("."); err != nil {
		t.Fatal("caller close invalidated pinned directory", err)
	}
	probe, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if writer, err := lockPrivateRunnerRoot(ctx, probe); err == nil {
		writer.Close()
		t.Fatal("inspection did not retain preparation exclusion")
	}
	called := false
	consume := func(context.Context, PreparedRecoveryInventory, []PreparedRecoveryFile) error {
		called = true
		return nil
	}
	if err = lease.WithFiles(ctx, uid, Manifest{}, RestorePolicy{}, consume); err == nil || called {
		t.Fatal("unprepared files reached handoff", err)
	}
	if err = lease.WithFiles(ctx, uid+1, Manifest{}, RestorePolicy{}, consume); !errors.Is(err, ErrManifest) || called {
		t.Fatal("foreign owner reached handoff", err)
	}
	lease.mu.Lock()
	_, err = lease.Inventory(ctx, Manifest{}, RestorePolicy{})
	if handoffErr := lease.WithFiles(ctx, uid, Manifest{}, RestorePolicy{}, consume); !errors.Is(handoffErr, ErrMaintenanceRunner) || called {
		t.Fatal("overlapping handoff accepted", handoffErr)
	}
	lease.mu.Unlock()
	if !errors.Is(err, ErrMaintenanceRunner) {
		t.Fatal("overlapping inspection accepted", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = lease.Inventory(ctx, Manifest{}, RestorePolicy{}); !errors.Is(err, ErrManifest) {
		t.Fatal("closed lease inspected output", err)
	}
	writer, err = lockPrivateRunnerRoot(ctx, probe)
	if err != nil {
		t.Fatal("close retained exclusion", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}
