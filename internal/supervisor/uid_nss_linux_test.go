//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeGuestNSSNameServiceEligibility(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	directory := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "nsswitch.conf")
	valid := []byte("passwd: files\ngroup: files\nshadow: files\nsubid: files\nhosts: files dns\n")
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := observeGuestUIDNameServiceEligibilityAt(context.Background(), directory); err != nil {
		t.Fatal("protected local-only NSS refused", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
		t.Fatal("observation changed configuration", err)
	}
	dynamic := []byte(strings.Replace(string(valid), "passwd: files", "passwd: files systemd", 1))
	if err := os.WriteFile(path, dynamic, 0600); err != nil {
		t.Fatal(err)
	}
	if err := observeGuestUIDNameServiceEligibilityAt(context.Background(), directory); !errors.Is(err, ErrPolicy) {
		t.Fatal("dynamic identity source accepted", err)
	}
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0777); err != nil {
		t.Fatal(err)
	}
	if err := observeGuestUIDNameServiceEligibilityAt(context.Background(), directory); !errors.Is(err, ErrPolicy) {
		t.Fatal("writable directory accepted", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := observeGuestUIDNameServiceEligibilityAt(context.Background(), directory); !errors.Is(err, ErrPolicy) {
		t.Fatal("symlink accepted", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != string(valid) {
		t.Fatal("refusal changed target", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observeGuestUIDNameServiceEligibilityAt(ctx, directory); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
