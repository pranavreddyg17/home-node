//go:build linux

package install

import (
	"context"
	"encoding/json"
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
	for _, fault := range []string{"none", "replace-source", "change-source", "replace-intent", "writable-source", "alias-source", "stage-corrupt", "stage-occupied"} {
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
			err = e.withGuestUIDAllocatorHostSource(ctx, intent, func(_ context.Context, file *os.File, check func() error) error {
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
				if fault == "none" || fault == "stage-corrupt" || fault == "stage-occupied" {
					directory, err := e.host.OpenRoot("etc")
					if err != nil {
						return err
					}
					defer directory.Close()
					stagePath := filepath.Join(host, "etc/.homenode-login-defs.stage")
					if fault == "stage-occupied" {
						if err := os.WriteFile(stagePath, []byte(proposal.Contents), 0600); err != nil {
							return err
						}
					}
					observations := 0
					stage, err := e.stageGuestUIDAllocation(ctx, directory, file, intent, func(context.Context) error {
						observations++
						if fault == "stage-corrupt" && observations == 2 {
							data := []byte(proposal.Contents)
							data[0] = '!'
							if err := os.WriteFile(stagePath, data, 0600); err != nil {
								return err
							}
						}
						return check()
					})
					if fault == "none" {
						if err != nil || stage.Version != 1 || stage.Inode == 0 || stage.SourceInode == 0 || stage.Inode == stage.SourceInode || stage.Bytes != int64(len(proposal.Contents)) {
							t.Fatal("allocator staging authority incomplete", err)
						}
						data, readErr := os.ReadFile(stagePath)
						if readErr != nil || string(data) != proposal.Contents {
							t.Fatal("allocator staging bytes changed", readErr)
						}
						record, readErr := e.journalRoot.ReadFile("guest-uid-allocation-stage.json")
						var saved guestUIDAllocationStage
						if readErr != nil || json.Unmarshal(record, &saved) != nil || saved != stage {
							t.Fatal("allocator staging receipt does not bind returned authority", readErr)
						}
					} else {
						if stage != (guestUIDAllocationStage{}) || err == nil {
							t.Fatal("failed allocator staging returned authority", err)
						}
						if _, recordErr := e.journalRoot.Lstat("guest-uid-allocation-stage.json"); !os.IsNotExist(recordErr) {
							t.Fatal("failed allocator staging committed authority", recordErr)
						}
					}
					if err != nil {
						return err
					}
				}
				return check()
			})
			if fault == "none" {
				if err != nil || !called {
					t.Fatal("qualified allocator source refused", err)
				}
			} else if fault == "stage-occupied" {
				if !errors.Is(err, os.ErrExist) {
					t.Fatal("foreign allocator stage adopted", err)
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
