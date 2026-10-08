//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootGuestIdentityHostSourceRetention(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, fault := range []string{"none", "writable-source", "alias-source", "replace-source", "change-source", "replace-parent"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			parent := filepath.Join(host, "etc")
			path := filepath.Join(parent, "nsswitch.conf")
			data := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			proposal, err := planGuestIdentityNameServices(data)
			if err != nil {
				t.Fatal(err)
			}
			owner := strings.Repeat("a", 32)
			if err := e.commitGuestIdentityNameServices(context.Background(), owner, data, proposal); err != nil {
				t.Fatal(err)
			}
			intent := guestIdentityNameServiceIntent{Version: 1, OwnerID: owner, Original: string(data), Proposal: proposal}
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
			err = e.withGuestIdentityHostSource(context.Background(), intent, func(ctx context.Context, source *os.File) error {
				called = true
				switch fault {
				case "replace-source":
					if err := os.Rename(path, path+".original"); err != nil {
						return err
					}
					return os.WriteFile(path, data, 0644)
				case "change-source":
					changed := append([]byte(nil), data...)
					changed[0] ^= 1
					return os.WriteFile(path, changed, 0644)
				case "replace-parent":
					if err := os.Rename(parent, parent+".original"); err != nil {
						return err
					}
					return os.Mkdir(parent, 0755)
				}
				return ctx.Err()
			})
			if fault == "none" {
				if err != nil || !called {
					t.Fatal("qualified original refused", err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					e.mu.Lock()
					err = e.applyGuestIdentityNameServicesLocked(context.Background(), intent, func(context.Context) error { return nil })
					e.mu.Unlock()
					if err != nil {
						t.Fatal("identity transaction or exact retry refused", attempt, err)
					}
				}
				current, err := os.ReadFile(path)
				original, originalErr := os.ReadFile(filepath.Join(parent, ".homenode-nsswitch.stage"))
				if err != nil || originalErr != nil || string(current) != proposal.Contents || string(original) != string(data) {
					t.Fatal("identity transaction lost retained original", err, originalErr)
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("source drift accepted", fault, err)
			}
			if (fault == "writable-source" || fault == "alias-source") && called {
				t.Fatal("invalid source reached consumer")
			}
		})
	}
}
