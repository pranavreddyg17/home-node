//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageConfigurationSourceRefusesReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	for _, name := range []string{"runtime-policy.json", "services.env"} {
		mode := os.FileMode(0644)
		if name == "runtime-policy.json" {
			mode = 0600
		}
		path := filepath.Join(directory, name)
		original := "owned configuration\n"
		if err := os.WriteFile(path, []byte(original), mode); err != nil {
			t.Fatal(err)
		}
		if err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error { return check() }); err != nil {
			t.Fatal(err)
		}
		err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(original), mode); err != nil {
				return err
			}
			if err := check(); !errors.Is(err, ErrConflict) {
				t.Fatal("identical replacement admitted", err)
			}
			return nil
		})
		if !errors.Is(err, ErrConflict) {
			t.Fatal("replacement scope succeeded", err)
		}
		for _, p := range []string{path, path + ".original"} {
			data, err := os.ReadFile(p)
			if err != nil || string(data) != original {
				t.Fatal("replacement evidence altered", err)
			}
		}
	}
}
