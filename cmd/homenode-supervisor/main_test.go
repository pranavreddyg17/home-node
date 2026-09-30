package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestIndependentFloorCannotBeLoweredByRecordedVersion(t *testing.T) {
	for _, test := range []struct {
		configured int64
		stored     string
		want       int64
	}{
		{4, "1", 4}, {4, "5", 5}, {4, "4", 4}, {4, "0", 0}, {4, "", 0}, {4, "-1", 0}, {0, "5", 0}, {4, "05", 0},
	} {
		got, err := raiseCatalogFloor(test.configured, test.stored)
		if test.want == 0 {
			if err == nil {
				t.Fatal("invalid floor admitted", test)
			}
		} else if err != nil || got != test.want {
			t.Fatal(test, got, err)
		}
	}
}
func TestRootProtectedConfigurationReader(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "floor")
	if err := os.WriteFile(name, []byte("4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readProtected(name, 32)
	if err != nil || string(data) != "4\n" {
		t.Fatal(data, err)
	}
	if _, err = readProtected(name, 1); err == nil {
		t.Fatal("size bound ignored")
	}
	if err = os.Chmod(name, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err = readProtected(name, 32); err == nil {
		t.Fatal("writable configuration admitted")
	}
	if err = os.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(name, link); err != nil {
		t.Fatal(err)
	}
	if _, err = readProtected(link, 32); err == nil {
		t.Fatal("symlink configuration admitted")
	}
	fifo := filepath.Join(dir, "fifo")
	if err = unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readProtected(fifo, 32); err == nil {
		t.Fatal("nonregular configuration admitted")
	}
}
