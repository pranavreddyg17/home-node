//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRecoveryPublicationComposesCommittedContentAndGuard(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root temporary namespace fixture")
	}
	for _, fault := range []string{"none", "guard-refuses", "content-drift", "uncommitted"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			destination := t.TempDir()
			stage := ".recovery-management.publish"
			stagePath := filepath.Join(destination, stage)
			data := []byte("restored management fixture")
			if err := os.WriteFile(stagePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(stagePath)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			directory, err := os.Open(destination)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			var stat unix.Stat_t
			if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			recovery := recoveryIntent{Version: 1, ConfigurationID: strings.Repeat("a", 32), ConfigurationDigest: strings.Repeat("b", 64)}
			recovery.Recovery.ManagementSHA256 = hex.EncodeToString(hash[:])
			recovery.Recovery.ManagementBytes = int64(len(data))
			encoded, err := json.Marshal(recovery)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.commitRecoveryIntent(context.Background(), encoded); err != nil {
				t.Fatal(err)
			}
			intent := recoveryPublicationIntent{Version: 1, ConfigurationID: recovery.ConfigurationID, ConfigurationDigest: recovery.ConfigurationDigest, RecoveryIntentSHA256: digest(encoded), FileName: "management.db", ContentSHA256: recovery.Recovery.ManagementSHA256, Identity: recoveryPublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Bytes: stat.Size, UID: stat.Uid, GID: stat.Gid}}
			if fault != "uncommitted" {
				if err := e.commitRecoveryPublicationIntent(context.Background(), intent); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "content-drift" {
				changed := append([]byte(nil), data...)
				changed[0] ^= 1
				if err := os.WriteFile(stagePath, changed, 0600); err != nil {
					t.Fatal(err)
				}
			}
			checks := 0
			guard := func(context.Context) error {
				checks++
				if fault == "guard-refuses" {
					return ErrConflict
				}
				return nil
			}
			e.mu.Lock()
			err = e.publishRecoveryWithIntent(context.Background(), directory, file, stage, intent, recovery, guard)
			e.mu.Unlock()
			final := filepath.Join(destination, "management.db")
			if fault == "none" {
				if err != nil || checks != 3 {
					t.Fatal("combined publication refused", checks, err)
				}
				if err := verifyRecoveryPublicationDescriptor(context.Background(), file, intent); err != nil {
					t.Fatal("published descriptor lost authority", err)
				}
				published, err := os.Stat(final)
				pinned, statErr := file.Stat()
				if err != nil || statErr != nil || !os.SameFile(published, pinned) {
					t.Fatal("final name lost retained inode", err, statErr)
				}
				if _, err := os.Stat(stagePath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("stage remained after publication", err)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe publication admitted")
				}
				if _, err := os.Stat(final); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("refusal published a destination", err)
				}
				if _, err := os.Stat(stagePath); err != nil {
					t.Fatal("refusal removed staging", err)
				}
				if fault == "uncommitted" && checks != 0 {
					t.Fatal("guard ran without committed intent")
				}
			}
		})
	}
}
