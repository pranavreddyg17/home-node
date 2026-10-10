//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageRuntimePreparationCreatesButNeverRepairs(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	if err := os.Mkdir(filepath.Join(host, "run"), 0755); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, journalDir)
	defer engine.Close()
	ctx := context.Background()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := engine.prepareGuestStorageChannelRuntime(ctx, guard); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(host, "run", "homenode")
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareGuestStorageChannelRuntime(ctx, guard); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("retry replaced runtime inode", err)
	}
	if err := os.Chmod(path, 0775); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareGuestStorageChannelRuntime(ctx, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign runtime metadata admitted", err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode().Perm() != 0775 {
		t.Fatal("foreign metadata repaired", err)
	}
}
