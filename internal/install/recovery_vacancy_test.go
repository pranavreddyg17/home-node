package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryDirectoryVacancyPreservesExistingEntries(t *testing.T) {
	for _, entry := range []string{"management.db", "management.db-wal", "management.db-shm", "management.db-journal", "guest.raw", "unknown"} {
		t.Run(entry, func(t *testing.T) {
			host, _ := roots(t)
			name := "var/lib/homenode/control"
			directory := filepath.Join(host, name)
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(host)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err = recoveryDirectoryVacant(context.Background(), root, name); err != nil {
				t.Fatal("empty refused", err)
			}
			path := filepath.Join(directory, entry)
			if err = os.WriteFile(path, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = recoveryDirectoryVacant(context.Background(), root, name); !errors.Is(err, ErrConflict) {
				t.Fatal("occupied admitted", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "preserve" {
				t.Fatal("existing entry changed", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err = recoveryDirectoryVacant(ctx, root, name); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if err = recoveryDirectoryVacant(context.Background(), root, "foreign"); !errors.Is(err, ErrPlan) {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryDirectoryVacancyRejectsSymlinkAndMissingDestinations(t *testing.T) {
	for _, name := range []string{"var/lib/homenode/control", "var/lib/homenode/supervisor", "var/lib/homenode/volumes"} {
		t.Run(filepath.Base(name), func(t *testing.T) {
			host, _ := roots(t)
			root, err := os.OpenRoot(host)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err = recoveryDirectoryVacant(context.Background(), root, name); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("missing destination admitted", err)
			}
			path := filepath.Join(host, name)
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			if err = os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if err = recoveryDirectoryVacant(context.Background(), root, name); !errors.Is(err, ErrConflict) {
				t.Fatal("symlink destination admitted", err)
			}
			if entries, readErr := os.ReadDir(target); readErr != nil || len(entries) != 0 {
				t.Fatal("symlink target changed", readErr)
			}
			if info, statErr := os.Lstat(path); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("refused symlink replaced", statErr)
			}
			if err = os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("foreign destination"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = recoveryDirectoryVacant(context.Background(), root, name); !errors.Is(err, ErrConflict) {
				t.Fatal("file destination admitted", err)
			}
			if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "foreign destination" {
				t.Fatal("foreign file changed", readErr)
			}
		})
	}
}
