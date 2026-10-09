//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageImageParentReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	fixture := t.TempDir()
	path := filepath.Join(fixture, "var/lib/homenode/images")
	if err := os.MkdirAll(path, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 993); err != nil {
		t.Fatal(err)
	}
	host, err := os.OpenRoot(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	e := &Engine{host: host}
	ctx := context.Background()
	check := func(ctx context.Context) error { return ctx.Err() }
	if err := e.withGuestStorageImageParent(ctx, 993, check, func(root *os.Root, parent *os.File, guard func(context.Context) error) error {
		return guard(ctx)
	}); err != nil {
		t.Fatal("qualified parent refused", err)
	}
	if err := e.withGuestStorageImageParent(ctx, 993, check, func(root *os.Root, parent *os.File, guard func(context.Context) error) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0710); err != nil {
			return err
		}
		if err := os.Chown(path, 0, 993); err != nil {
			return err
		}
		if err := guard(ctx); !errors.Is(err, ErrConflict) {
			t.Fatal("identical parent replacement admitted", err)
		}
		return nil
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("final parent guard admitted replacement", err)
	}
	for _, path := range []string{path, path + ".original"} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() || info.Mode().Perm() != 0710 {
			t.Fatal("parent refusal modified fixture", err)
		}
	}
}
