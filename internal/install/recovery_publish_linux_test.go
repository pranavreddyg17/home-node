//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRecoveryPublicationDoesNotAdoptOccupiedOrUnqualifiedFiles(t *testing.T) {
	for _, fault := range []string{"none", "occupied", "wrong-inode", "mode-drift", "alias", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			stage, final := ".recovery-management.publish", "management.db"
			stagePath, finalPath := filepath.Join(root, stage), filepath.Join(root, final)
			data := []byte("qualified restored management fixture")
			if err := os.WriteFile(stagePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(stagePath)
			if err != nil {
				t.Fatal(err)
			}
			var stat unix.Stat_t
			if err := unix.Lstat(stagePath, &stat); err != nil {
				t.Fatal(err)
			}
			identity := recoveryPublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Bytes: stat.Size, UID: stat.Uid, GID: stat.Gid}
			ctx := context.Background()
			switch fault {
			case "occupied":
				// Even identical bytes do not establish installer ownership.
				if err := os.WriteFile(finalPath, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong-inode":
				identity.Inode++
			case "mode-drift":
				if err := os.Chmod(stagePath, 0640); err != nil {
					t.Fatal(err)
				}
			case "alias":
				if err := os.Link(stagePath, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			var occupied os.FileInfo
			if fault == "occupied" {
				occupied, err = os.Stat(finalPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			directory, err := os.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			err = publishRecoveryFile(ctx, directory, stage, final, identity)
			if fault == "none" {
				if err != nil {
					t.Fatal("qualified publication refused", err)
				}
				published, err := os.Stat(finalPath)
				if err != nil || !os.SameFile(before, published) {
					t.Fatal("publication lost identity", err)
				}
				if _, err := os.Stat(stagePath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("stage retained after rename", err)
				}
				if err := publishRecoveryFile(ctx, directory, stage, final, identity); err != nil {
					t.Fatal("exact final inode could not retry", err)
				}
				foreign := identity
				foreign.Inode++
				if err := publishRecoveryFile(ctx, directory, stage, final, foreign); !errors.Is(err, ErrConflict) {
					t.Fatal("foreign final inode adopted", err)
				}
			} else {
				if err == nil {
					t.Fatal("unqualified publication admitted")
				}
				if fault == "occupied" && !errors.Is(err, os.ErrExist) || fault == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal("refusal cause lost", err)
				}
				retained, err := os.Stat(stagePath)
				if err != nil || !os.SameFile(before, retained) {
					t.Fatal("refusal changed stage identity", err)
				}
				if fault == "occupied" {
					retained, err := os.Stat(finalPath)
					if err != nil || !os.SameFile(occupied, retained) {
						t.Fatal("occupied destination replaced", err)
					}
				} else if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("refusal created destination", err)
				}
			}
			retainedPath := stagePath
			if fault == "none" {
				retainedPath = finalPath
			}
			contents, err := os.ReadFile(retainedPath)
			if err != nil || string(contents) != string(data) {
				t.Fatal("publication changed data", err)
			}
			if fault == "occupied" {
				contents, err := os.ReadFile(finalPath)
				if err != nil || string(contents) != string(data) {
					t.Fatal("refusal changed occupied data", err)
				}
			}
		})
	}
}
