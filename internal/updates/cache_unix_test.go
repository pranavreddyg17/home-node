//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMetadataCacheDurabilityAdmission(t *testing.T) {
	for _, scenario := range []string{"valid", "public-directory", "writable-metadata", "missing-role", "symlink", "hardlink", "fifo", "oversized", "temporary", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"root.json", "timestamp.json", "snapshot.json", "targets.json"} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte("fixture metadata"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			filename := filepath.Join(directory, "targets.json")
			var err error
			switch scenario {
			case "public-directory":
				err = os.Chmod(directory, 0755)
			case "writable-metadata":
				err = os.Chmod(filename, 0666)
			case "missing-role":
				err = os.Remove(filename)
			case "symlink":
				err = os.Remove(filename)
				if err == nil {
					err = os.Symlink("root.json", filename)
				}
			case "hardlink":
				err = os.Remove(filename)
				if err == nil {
					err = os.Link(filepath.Join(directory, "root.json"), filename)
				}
			case "fifo":
				err = os.Remove(filename)
				if err == nil {
					err = unix.Mkfifo(filename, 0600)
				}
			case "oversized":
				err = os.Truncate(filename, maxMetadataDownload+1)
			case "temporary":
				err = os.WriteFile(filepath.Join(directory, "tuf_tmp-uncommitted"), []byte("incomplete"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			err = syncMetadataCache(ctx, root)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsafe/incomplete cache accepted")
			}
			if scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
