//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootReservedStorageParentObservation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	base := t.TempDir()
	paths := []string{filepath.Join(base, "images"), filepath.Join(base, "volumes"), filepath.Join(base, "channels")}
	for i, path := range paths {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		gid := 994
		if i == 2 {
			gid = 995
		}
		if err := os.Chown(path, 0, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0710); err != nil {
			t.Fatal(err)
		}
	}
	check := func() error {
		return ObserveReservedStorageParents(context.Background(), paths[0], paths[1], paths[2], 994, 995)
	}
	if err := check(); err != nil {
		t.Fatal("qualified parents refused", err)
	}
	if err := os.Chmod(paths[1], 0770); err != nil {
		t.Fatal(err)
	}
	if err := check(); !errors.Is(err, ErrPolicy) {
		t.Fatal("writable volume parent admitted", err)
	}
	info, err := os.Lstat(paths[1])
	if err != nil || info.Mode().Perm() != 0770 {
		t.Fatal("observer changed installed ownership", err)
	}
	if err := os.Chmod(paths[1], 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(paths[0], paths[0]+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(paths[0]+".original", paths[0]); err != nil {
		t.Fatal(err)
	}
	if err := check(); err == nil {
		t.Fatal("symlink parent admitted")
	}
}
