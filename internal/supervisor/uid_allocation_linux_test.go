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

func TestNativeGuestAutomaticUIDAllocationEligibility(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	directory := filepath.Join(t.TempDir(), "etc")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "login.defs")
	valid := []byte("UID_MIN 1000\nUID_MAX 60000\nSYS_UID_MIN 100\nSYS_UID_MAX 999\nSUB_UID_MIN 100000\nSUB_UID_MAX 600100000\n")
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	pool := GuestUIDPool{First: 2000000000, Last: 2000000001}
	observe := func(ctx context.Context) error {
		return observeProtectedGuestIdentityConfig(ctx, directory, "login.defs", func(data []byte) error { return validateGuestUIDAutomaticAllocation(ctx, pool, data) })
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := observe(context.Background()); err != nil {
		t.Fatal("protected disjoint defaults refused", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
		t.Fatal("observation changed defaults", err)
	}
	conflicting := []byte(strings.Replace(string(valid), "SUB_UID_MAX 600100000", "SUB_UID_MAX 2000000001", 1))
	if err := os.WriteFile(path, conflicting, 0600); err != nil {
		t.Fatal(err)
	}
	if err := observe(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("overlapping allocator accepted", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(conflicting) {
		t.Fatal("conflicting host defaults repaired", err)
	}
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if err := observe(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("writable allocator defaults accepted", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0777); err != nil {
		t.Fatal(err)
	}
	if err := observe(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("writable directory accepted", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "preserved-defaults")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("preserved-defaults", path); err != nil {
		t.Fatal(err)
	}
	if err := observe(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("symlink defaults accepted", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != string(valid) {
		t.Fatal("symlink target changed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observe(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
