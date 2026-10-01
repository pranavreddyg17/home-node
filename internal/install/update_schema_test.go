package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/updates"
)

func TestAcquisitionDiscardsPackageWhenLiveSchemaChanges(t *testing.T) {
	for _, scenario := range []string{"unchanged", "changed", "state-closed", "canceled", "failed-download"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			store, err := state.Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			filename := filepath.Join(t.TempDir(), "verified.deb")
			if err = os.WriteFile(filename, []byte("fixture"), 0400); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("download fixture failure")
			result, err := acquireWithObservedSchema(ctx, store, func(schema int) (*updates.AcquiredRelease, error) {
				if schema != 4 {
					t.Fatal("wrong initial observation", schema)
				}
				switch scenario {
				case "changed":
					if _, err := store.DB.Exec("PRAGMA user_version=5"); err != nil {
						t.Fatal(err)
					}
				case "state-closed":
					if err := store.Close(); err != nil {
						t.Fatal(err)
					}
				case "canceled":
					cancel()
				case "failed-download":
					return &updates.AcquiredRelease{Package: file}, failure
				}
				return &updates.AcquiredRelease{Package: file}, nil
			})
			if scenario == "unchanged" {
				if err != nil || result == nil || result.Package != file {
					t.Fatal("valid acquisition discarded", err)
				}
				if _, err = file.Stat(); err != nil {
					t.Fatal("valid descriptor closed", err)
				}
			} else {
				if err == nil || result != nil {
					t.Fatal("stale result returned", err)
				}
				if _, statErr := file.Stat(); !errors.Is(statErr, os.ErrClosed) {
					t.Fatal("discarded descriptor still open", statErr)
				}
				if scenario == "failed-download" && !errors.Is(err, failure) {
					t.Fatal(err)
				}
				if scenario == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
		})
	}
}
