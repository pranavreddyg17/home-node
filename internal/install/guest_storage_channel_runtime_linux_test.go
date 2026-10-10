//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageChannelRuntimeRefusesDirectoryReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	path := filepath.Join(host, "run", "homenode")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, journalDir)
	defer engine.Close()
	ctx := context.Background()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := engine.withGuestStorageChannelRuntime(ctx, guard, func(_ *os.Root, _ *os.File, check func(context.Context) error) error { return check(ctx) }); err != nil {
		t.Fatal(err)
	}
	err := engine.withGuestStorageChannelRuntime(ctx, guard, func(_ *os.Root, _ *os.File, check func(context.Context) error) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0755); err != nil {
			return err
		}
		return check(ctx)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("matching replacement runtime directory admitted", err)
	}
	for _, name := range []string{path, path + ".original"} {
		if info, err := os.Lstat(name); err != nil || !info.IsDir() || info.Mode().Perm() != 0755 {
			t.Fatal("runtime directory evidence altered", name, err)
		}
	}
}

func TestRootGuestStorageChannelRuntimeRefusesWritableParentBeforeConsumer(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	path := filepath.Join(host, "run", "homenode")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0775); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, journalDir)
	defer engine.Close()
	called := false
	err := engine.withGuestStorageChannelRuntime(context.Background(), func(ctx context.Context) error { return ctx.Err() }, func(_ *os.Root, _ *os.File, _ func(context.Context) error) error { called = true; return nil })
	if !errors.Is(err, ErrConflict) || called {
		t.Fatal("writable runtime directory reached consumer", called, err)
	}
}
