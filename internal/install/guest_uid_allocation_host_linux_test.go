//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestRootGuestUIDAllocatorSourceRejectsDrift(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, fault := range []string{"none", "replace-source", "change-source", "replace-intent", "writable-source", "alias-source"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			ctx := context.Background()
			original := "# allocator fixture\nUID_MIN 1000\nUID_MAX 60000\n"
			selection := guestUIDAllocatorRanges{NormalFirst: 1000, NormalLast: 60000, SystemFirst: 100, SystemLast: 999, SubordinateFirst: 100000, SubordinateLast: 600100000}
			pool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
			proposal, err := planGuestUIDAllocatorConfiguration(ctx, []byte(original), pool, selection)
			if err != nil {
				t.Fatal(err)
			}
			intent := guestUIDAllocationIntent{Version: 1, OwnerID: strings.Repeat("a", 32), First: pool.First, Last: pool.Last, Selection: selection, Original: original, Proposal: proposal}
			if err := e.commitGuestUIDAllocationIntent(ctx, intent); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(host, "etc/login.defs")
			if err := os.WriteFile(path, []byte(original), 0644); err != nil {
				t.Fatal(err)
			}
			if fault == "writable-source" {
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "alias-source" {
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			err = e.withGuestUIDAllocatorHostSource(ctx, intent, func(_ context.Context, _ *os.File, check func() error) error {
				called = true
				switch fault {
				case "replace-source":
					if err := os.Rename(path, path+".held"); err != nil {
						return err
					}
					if err := os.WriteFile(path, []byte(original), 0644); err != nil {
						return err
					}
				case "change-source":
					if err := os.WriteFile(path, []byte(strings.Replace(original, "60000", "60001", 1)), 0644); err != nil {
						return err
					}
				case "replace-intent":
					record := filepath.Join(journal, "guest-uid-allocation-intent.json")
					data, err := os.ReadFile(record)
					if err != nil {
						return err
					}
					if err := os.Rename(record, record+".held"); err != nil {
						return err
					}
					if err := os.WriteFile(record, data, 0600); err != nil {
						return err
					}
				}
				return check()
			})
			if fault == "none" {
				if err != nil || !called {
					t.Fatal("qualified allocator source refused", err)
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("allocator source drift retained authority", fault, err)
			}
			if (fault == "writable-source" || fault == "alias-source") && called {
				t.Fatal("invalid allocator source reached consumer")
			}
		})
	}
}
