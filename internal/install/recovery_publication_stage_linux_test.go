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

	"github.com/pranavreddyg17/home-node/internal/backup"
	"golang.org/x/sys/unix"
)

func TestRootRecoveryPublicationJournalsBeforeOwnershipTransfer(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, phase := range []string{"recovery-publication-intent", "recovery-publication-owned"} {
		t.Run(phase, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			data := []byte("disconnected restored management fixture")
			sourcePath := filepath.Join(t.TempDir(), "management.db")
			if err := os.WriteFile(sourcePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			hash := sha256.Sum256(data)
			source := backup.PreparedRecoveryFile{Name: "management.db", Bytes: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), File: file}
			recovery := recoveryIntent{Version: 1, ConfigurationID: strings.Repeat("a", 32), ConfigurationDigest: strings.Repeat("b", 64)}
			recovery.Recovery.ManagementSHA256, recovery.Recovery.ManagementBytes = source.SHA256, source.Bytes
			encoded, err := json.Marshal(recovery)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.commitRecoveryIntent(context.Background(), encoded); err != nil {
				t.Fatal(err)
			}
			destinationPath := t.TempDir()
			destination, err := os.OpenRoot(destinationPath)
			if err != nil {
				t.Fatal(err)
			}
			defer destination.Close()
			interrupted := errors.New("ownership checkpoint interrupted")
			e.checkpoint = func(point, name string) error {
				if point == phase && name == "management.db" {
					return interrupted
				}
				return nil
			}
			guard := func(context.Context) error { return nil }
			e.mu.Lock()
			intent, err := e.prepareRecoveryPublicationFile(context.Background(), destination, source, recovery, 801, 801, guard)
			e.mu.Unlock()
			if !errors.Is(err, interrupted) || intent.Identity.Inode != 0 {
				t.Fatal("interruption lost or intent leaked", err, intent)
			}
			record, err := e.journalRoot.ReadFile("recovery-management-publication.json")
			if err != nil {
				t.Fatal("ownership intent missing", err)
			}
			var saved recoveryPublicationIntent
			if err := json.Unmarshal(record, &saved); err != nil || validateRecoveryPublicationIntent(saved) != nil {
				t.Fatal("invalid retained ownership intent", err)
			}
			stagePath := filepath.Join(destinationPath, ".recovery-management.publish")
			var stat unix.Stat_t
			if err := unix.Lstat(stagePath, &stat); err != nil || stat.Ino != saved.Identity.Inode || uint64(stat.Dev) != saved.Identity.Device || stat.Size != source.Bytes || stat.Mode != unix.S_IFREG|0600 || stat.Nlink != 1 {
				t.Fatal("interrupted staging identity lost", err)
			}
			if phase == "recovery-publication-intent" && stat.Uid != 0 || phase == "recovery-publication-owned" && (stat.Uid != 801 || stat.Gid != 801) {
				t.Fatal("ownership effect preceded or missed checkpoint", stat.Uid, stat.Gid)
			}
			contents, err := os.ReadFile(stagePath)
			if err != nil || string(contents) != string(data) {
				t.Fatal("interrupted bytes lost", err)
			}
			if _, err := os.Stat(filepath.Join(destinationPath, "management.db")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("staging interruption published data", err)
			}
			e.checkpoint = nil
			e.mu.Lock()
			_, err = e.prepareRecoveryPublicationFile(context.Background(), destination, source, recovery, 801, 801, guard)
			e.mu.Unlock()
			if !errors.Is(err, os.ErrExist) {
				t.Fatal("fresh preparation adopted interrupted staging", err)
			}
			retained, err := e.journalRoot.ReadFile("recovery-management-publication.json")
			if err != nil || string(record) != string(retained) {
				t.Fatal("refused retry changed ownership intent", err)
			}
			// Equal-size corruption must refuse before either ownership or
			// the consumer changes the recorded object.
			corrupt := append([]byte(nil), data...)
			corrupt[0] ^= 1
			if err := os.WriteFile(stagePath, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			consumed := false
			e.mu.Lock()
			err = e.withResumedRecoveryPublication(context.Background(), destination, saved, recovery, guard, func(context.Context, *os.File) error {
				consumed = true
				return nil
			})
			e.mu.Unlock()
			if !errors.Is(err, ErrConflict) || consumed {
				t.Fatal("corrupted committed stage resumed", err)
			}
			var unchanged unix.Stat_t
			if err := unix.Lstat(stagePath, &unchanged); err != nil || unchanged.Ino != stat.Ino || unchanged.Uid != stat.Uid || unchanged.Gid != stat.Gid {
				t.Fatal("refused corruption changed ownership", err)
			}
			if err := os.WriteFile(stagePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			e.mu.Lock()
			err = e.withResumedRecoveryPublication(context.Background(), destination, saved, recovery, guard, func(ctx context.Context, resumed *os.File) error {
				consumed = true
				return verifyRecoveryPublicationDescriptor(ctx, resumed, saved)
			})
			e.mu.Unlock()
			if err != nil || !consumed {
				t.Fatal("committed staging could not resume ownership", err)
			}
			if err := unix.Lstat(stagePath, &stat); err != nil || stat.Ino != saved.Identity.Inode || stat.Uid != 801 || stat.Gid != 801 {
				t.Fatal("resume changed inode or missed ownership", err)
			}
			if _, err := os.Stat(filepath.Join(destinationPath, "management.db")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("ownership resume published data", err)
			}
		})
	}
}
