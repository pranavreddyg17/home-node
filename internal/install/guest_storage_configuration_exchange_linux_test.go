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

func TestRootGuestStorageConfigurationExchangeAndInterruptedRetry(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, name := range []string{"runtime-policy.json", "services.env"} {
		mode := os.FileMode(0644)
		pendingName := ".homenode-services-env.stage"
		if name == "runtime-policy.json" {
			mode = 0600
			pendingName = ".homenode-runtime-policy.stage"
		}
		for _, fault := range []string{"none", "after-exchange", "foreign-current", "alias-stage", "corrupt-stage", "cancel"} {
			t.Run(name+"/"+fault, func(t *testing.T) {
				parent := t.TempDir()
				if err := os.Chmod(parent, 0755); err != nil {
					t.Fatal(err)
				}
				final, pending := filepath.Join(parent, name), filepath.Join(parent, pendingName)
				original, desired := []byte("passwd: files systemd\n"), []byte("passwd: files\n")
				if err := os.WriteFile(final, original, mode); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(pending, desired, 0600); err != nil {
					t.Fatal(err)
				}
				var sourceStat, stageStat unix.Stat_t
				if err := unix.Lstat(final, &sourceStat); err != nil {
					t.Fatal(err)
				}
				if err := unix.Lstat(pending, &stageStat); err != nil {
					t.Fatal(err)
				}
				stage := guestStorageConfigurationStageFile{Name: pendingName, SHA256: digest(desired), SourceDevice: uint64(sourceStat.Dev), SourceInode: sourceStat.Ino, Device: uint64(stageStat.Dev), Inode: stageStat.Ino, Bytes: int64(len(desired))}
				switch fault {
				case "foreign-current":
					if err := os.Rename(final, final+".original"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(final, original, mode); err != nil {
						t.Fatal(err)
					}
				case "alias-stage":
					if err := os.Link(pending, pending+".alias"); err != nil {
						t.Fatal(err)
					}
				case "corrupt-stage":
					corrupt := append([]byte(nil), desired...)
					corrupt[0] ^= 1
					if err := os.WriteFile(pending, corrupt, 0600); err != nil {
						t.Fatal(err)
					}
				}
				beforeFinal, _ := os.Lstat(final)
				beforePending, _ := os.Lstat(pending)
				directory, err := os.Open(parent)
				if err != nil {
					t.Fatal(err)
				}
				defer directory.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				interrupted := errors.New("exchange acknowledgement interrupted")
				calls := 0
				guard := func(context.Context) error {
					calls++
					if fault == "cancel" {
						cancel()
					}
					if fault == "after-exchange" && calls == 3 {
						return interrupted
					}
					return nil
				}
				err = exchangeGuestStorageConfigurationFile(ctx, directory, stage, original, desired, guard)
				if fault == "none" || fault == "after-exchange" {
					if fault == "none" && err != nil || fault == "after-exchange" && !errors.Is(err, interrupted) {
						t.Fatal("exchange result", err)
					}
					if err := exchangeGuestStorageConfigurationFile(context.Background(), directory, stage, original, desired, func(context.Context) error { return nil }); err != nil {
						t.Fatal("exact exchanged retry refused", err)
					}
					newFinal, _ := os.Lstat(final)
					newPending, _ := os.Lstat(pending)
					finalBytes, _ := os.ReadFile(final)
					pendingBytes, _ := os.ReadFile(pending)
					if newFinal == nil || newFinal.Mode().Perm() != mode {
						t.Fatal("incorrect final configuration mode")
					}
					if !os.SameFile(beforePending, newFinal) || !os.SameFile(beforeFinal, newPending) || string(finalBytes) != string(desired) || string(pendingBytes) != string(original) {
						t.Fatal("exchange lost recorded identities or original bytes")
					}
				} else {
					expected := ErrConflict
					if fault == "cancel" {
						expected = context.Canceled
					}
					if !errors.Is(err, expected) {
						t.Fatal("unsafe exchange accepted", err)
					}
					afterFinal, _ := os.Lstat(final)
					afterPending, _ := os.Lstat(pending)
					if !os.SameFile(beforeFinal, afterFinal) || !os.SameFile(beforePending, afterPending) {
						t.Fatal("refusal exchanged foreign namespace")
					}
				}
			})
		}
	}
}
