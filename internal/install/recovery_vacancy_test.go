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
