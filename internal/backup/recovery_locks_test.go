//go:build linux || darwin

package backup

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRecoveryRootLeasesExcludeOverlapAndReleaseOnFailure(t *testing.T) {
	open := func() *os.Root {
		path := t.TempDir()
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { root.Close() })
		return root
	}
	source, destination := open(), open()
	first, second, err := lockRecoveryRoots(context.Background(), source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if a, b, err := lockRecoveryRoots(context.Background(), source, destination); err == nil || a != nil || b != nil {
		t.Fatal("overlap acquired recovery roots", err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if a, b, err := lockRecoveryRoots(context.Background(), source, destination); err == nil || a != nil || b != nil {
		t.Fatal("occupied destination acquired", err)
	}
	// Failure on destination must release the source lease.
	probe, err := lockPrivateRunnerRoot(context.Background(), source)
	if err != nil {
		t.Fatal("source lease retained after destination refusal", err)
	}
	if err = probe.Close(); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	a, b, err := lockRecoveryRoots(context.Background(), source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(a.Close(), b.Close()); err != nil {
		t.Fatal(err)
	}
	if a, b, err := lockRecoveryRoots(context.Background(), source, source); !errors.Is(err, ErrManifest) || a != nil || b != nil {
		t.Fatal("same-root recovery lease accepted", err)
	}
}
