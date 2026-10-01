//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestUpdateClockPersistsAndRefusesRollback(t *testing.T) {
	root, directory := updateCacheFixture(t)
	ctx := context.Background()
	first := time.Now().UTC()
	if err := recordUpdateClock(ctx, root, first); err != nil {
		t.Fatal(err)
	}
	reopened, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = recordUpdateClock(ctx, reopened, first.Add(-time.Nanosecond)); !errors.Is(err, errUpdateClock) {
		t.Fatal("clock rollback accepted", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "clock"))
	if err != nil || string(data) != strconv.FormatInt(first.UnixNano(), 10)+"\n" {
		t.Fatal("rollback changed checkpoint", err)
	}
	if err = recordUpdateClock(ctx, reopened, first); err != nil {
		t.Fatal("same time rejected", err)
	}
	if err = recordUpdateClock(ctx, reopened, first.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateClockRefusesUnsafeOrLostEvidence(t *testing.T) {
	for _, scenario := range []string{"malformed", "public", "symlink", "hardlink", "pending", "missing-with-metadata", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, directory := updateCacheFixture(t)
			now := time.Now().UTC()
			name := filepath.Join(directory, "clock")
			var err error
			switch scenario {
			case "malformed":
				err = os.WriteFile(name, []byte("01\n"), 0600)
			case "public":
				err = os.WriteFile(name, []byte("1\n"), 0644)
			case "symlink":
				err = os.Symlink("target", name)
			case "hardlink":
				err = os.WriteFile(filepath.Join(directory, "target"), []byte("1\n"), 0600)
				if err == nil {
					err = os.Link(filepath.Join(directory, "target"), name)
				}
			case "pending":
				err = os.WriteFile(filepath.Join(directory, "clock.pending"), []byte("interrupted"), 0600)
			case "missing-with-metadata":
				err = os.WriteFile(filepath.Join(directory, "metadata", "timestamp.json"), []byte("retained"), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			if err = recordUpdateClock(ctx, root, now); err == nil {
				t.Fatal("unsafe clock evidence accepted")
			}
			if scenario == "canceled" {
				if _, err = os.Lstat(name); !os.IsNotExist(err) {
					t.Fatal("cancellation wrote checkpoint", err)
				}
			}
		})
	}
}
