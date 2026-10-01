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

func updateCacheFixture(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "metadata"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root, directory
}

func TestMetadataCacheExclusivePersistentLock(t *testing.T) {
	provisioned, _ := updateCacheFixture(t)
	first, err := lockMetadataCache(context.Background(), provisioned)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	original, err := first.lock.Stat()
	if err != nil {
		t.Fatal(err)
	}
	other, err := lockMetadataCache(context.Background(), provisioned)
	if err == nil {
		other.Close()
		t.Fatal("concurrent updater admitted")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := lockMetadataCache(context.Background(), provisioned)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	observed, err := second.lock.Stat()
	if err != nil || !os.SameFile(original, observed) {
		t.Fatal("lock inode changed", err)
	}
}

func TestMetadataCacheLockRefusesUnsafeProvisioning(t *testing.T) {
	for _, scenario := range []string{"parent", "metadata-mode", "metadata-link", "lock-mode", "lock-content", "lock-link", "lock-hardlink", "lock-fifo", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			provisioned, directory := updateCacheFixture(t)
			lock := filepath.Join(directory, "update.lock")
			var err error
			switch scenario {
			case "parent":
				err = os.Chmod(directory, 0755)
			case "metadata-mode":
				err = os.Chmod(filepath.Join(directory, "metadata"), 0755)
			case "metadata-link":
				err = os.Rename(filepath.Join(directory, "metadata"), filepath.Join(directory, "other"))
				if err == nil {
					err = os.Symlink("other", filepath.Join(directory, "metadata"))
				}
			case "lock-mode":
				err = os.WriteFile(lock, nil, 0644)
			case "lock-content":
				err = os.WriteFile(lock, []byte("old intent"), 0600)
			case "lock-link":
				err = os.Symlink("target", lock)
			case "lock-hardlink":
				err = os.WriteFile(filepath.Join(directory, "target"), nil, 0600)
				if err == nil {
					err = os.Link(filepath.Join(directory, "target"), lock)
				}
			case "lock-fifo":
				err = unix.Mkfifo(lock, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			cache, err := lockMetadataCache(ctx, provisioned)
			if cache != nil {
				cache.Close()
			}
			if err == nil {
				t.Fatal("unsafe provisioning admitted")
			}
			if scenario == "canceled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if _, err = os.Lstat(lock); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("canceled claim created lock", err)
				}
			}
		})
	}
}
