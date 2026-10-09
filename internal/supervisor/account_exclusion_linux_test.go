//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootReservedAccountScopeRefusesDirectoryReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "etc"), 0700); err != nil {
		t.Fatal(err)
	}
	host, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	err = withReservedAccountDirectory(context.Background(), host, func(ctx context.Context, guard func(context.Context) error) error {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := host.Rename("etc", "original"); err != nil {
			return err
		}
		if err := host.Mkdir("etc", 0700); err != nil {
			return err
		}
		if err := guard(ctx); !errors.Is(err, ErrPolicy) {
			t.Fatal("replacement admitted", err)
		}
		return nil
	})
	if !errors.Is(err, ErrPolicy) {
		t.Fatal("scope reported replacement success", err)
	}
	for _, name := range []string{"etc", "original"} {
		if _, err := host.Lstat(name); err != nil {
			t.Fatal("replacement evidence removed", name, err)
		}
	}
}

func TestReservedAccountScopeCancellationBeforeOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := withReservedAccountDirectory(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
