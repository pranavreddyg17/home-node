//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestRootGuestStorageConfigurationBatchPreflightAndInterruptedRetry(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, fault := range []string{"corrupt-environment", "between-exchanges"} {
		t.Run(fault, func(t *testing.T) {
			parent := t.TempDir()
			intent := guestStorageConfigurationIntent{Version: 1, SourcePolicy: []byte("old policy\n"), SourceEnvironment: []byte("old environment\n"), Policy: []byte("new policy\n"), Environment: []byte("new environment\n")}
			encoded, _ := json.Marshal(intent)
			stage := guestStorageConfigurationStage{Version: 1, IntentSHA256: digest(encoded)}
			finals := []string{"runtime-policy.json", "services.env"}
			pending := []string{".homenode-runtime-policy.stage", ".homenode-services-env.stage"}
			originals := [][]byte{intent.SourcePolicy, intent.SourceEnvironment}
			desired := [][]byte{intent.Policy, intent.Environment}
			for i := range finals {
				mode := os.FileMode(0644)
				if i == 0 {
					mode = 0600
				}
				if err := os.WriteFile(filepath.Join(parent, finals[i]), originals[i], mode); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(parent, pending[i]), desired[i], 0600); err != nil {
					t.Fatal(err)
				}
				var source, replacement unix.Stat_t
				if err := unix.Lstat(filepath.Join(parent, finals[i]), &source); err != nil {
					t.Fatal(err)
				}
				if err := unix.Lstat(filepath.Join(parent, pending[i]), &replacement); err != nil {
					t.Fatal(err)
				}
				stage.Files = append(stage.Files, guestStorageConfigurationStageFile{Name: pending[i], SourceDevice: uint64(source.Dev), SourceInode: source.Ino, Device: uint64(replacement.Dev), Inode: replacement.Ino, Bytes: int64(len(desired[i])), SHA256: digest(desired[i])})
			}
			before, _ := os.Stat(filepath.Join(parent, finals[0]))
			if fault == "corrupt-environment" {
				if err := os.WriteFile(filepath.Join(parent, pending[1]), []byte("unknown content\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			directory, err := os.Open(parent)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			interrupted := errors.New("interrupted after policy publication")
			guard := func(context.Context) error {
				data, err := os.ReadFile(filepath.Join(parent, finals[0]))
				if err != nil {
					return err
				}
				if fault == "between-exchanges" && string(data) == string(intent.Policy) {
					return interrupted
				}
				return nil
			}
			err = exchangeGuestStorageConfigurationBatch(context.Background(), directory, stage, intent, guard)
			if fault == "corrupt-environment" {
				after, _ := os.Stat(filepath.Join(parent, finals[0]))
				if !errors.Is(err, ErrConflict) || !os.SameFile(before, after) {
					t.Fatal("policy changed before full preflight", err)
				}
			} else {
				if !errors.Is(err, interrupted) {
					t.Fatal("interruption not observed", err)
				}
				if err := exchangeGuestStorageConfigurationBatch(context.Background(), directory, stage, intent, func(context.Context) error { return nil }); err != nil {
					t.Fatal("interrupted batch retry failed", err)
				}
				for i, name := range finals {
					data, err := os.ReadFile(filepath.Join(parent, name))
					if err != nil || string(data) != string(desired[i]) {
						t.Fatal("batch incomplete", err)
					}
				}
			}
		})
	}
}
