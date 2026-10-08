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

func TestRootGuestIdentityStagingRefusesDriftBeforeCommit(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, fault := range []string{"none", "source", "stage", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			original := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
			proposal, err := planGuestIdentityNameServices(original)
			if err != nil {
				t.Fatal(err)
			}
			intent := guestIdentityNameServiceIntent{Version: 1, OwnerID: strings.Repeat("a", 32), Original: string(original), Proposal: proposal}
			if err := e.commitGuestIdentityNameServices(context.Background(), intent.OwnerID, original, proposal); err != nil {
				t.Fatal(err)
			}
			parent := filepath.Join(host, "etc")
			sourcePath := filepath.Join(parent, "nsswitch.conf")
			if err := os.WriteFile(sourcePath, original, 0644); err != nil {
				t.Fatal(err)
			}
			source, err := os.Open(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			directory, err := os.OpenRoot(parent)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			guard := func(context.Context) error {
				calls++
				if calls == 2 {
					switch fault {
					case "source":
						data := append([]byte(nil), original...)
						data[0] ^= 1
						return os.WriteFile(sourcePath, data, 0644)
					case "stage":
						return os.WriteFile(filepath.Join(parent, ".homenode-nsswitch.stage"), []byte(strings.Repeat("x", len(proposal.Contents))), 0600)
					case "cancel":
						cancel()
					}
				}
				return nil
			}
			stage, err := e.stageGuestIdentityNameServices(ctx, directory, source, intent, guard)
			if fault == "none" {
				if err != nil || stage.Inode == 0 || stage.SourceInode == 0 {
					t.Fatal("qualified staging refused", err)
				}
				parentFile, err := directory.Open(".")
				if err != nil {
					t.Fatal(err)
				}
				defer parentFile.Close()
				for attempt := 0; attempt < 2; attempt++ {
					if err := e.publishGuestIdentityNameServices(ctx, parentFile, stage, intent, guard); err != nil {
						t.Fatal("record-bound publication or retry refused", attempt, err)
					}
				}
				current, err := os.ReadFile(sourcePath)
				retained, retainedErr := os.ReadFile(filepath.Join(parent, ".homenode-nsswitch.stage"))
				if err != nil || retainedErr != nil || string(current) != proposal.Contents || string(retained) != string(original) {
					t.Fatal("publication lost intended or retained original bytes", err, retainedErr)
				}
			} else {
				expected := ErrConflict
				if fault == "cancel" {
					expected = context.Canceled
				}
				if !errors.Is(err, expected) || stage.Inode != 0 {
					t.Fatal("drift exposed staging authority", err)
				}
				if _, err := os.Stat(filepath.Join(journal, "guest-identity-nss-stage.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed staging committed identity", err)
				}
			}
		})
	}
}
