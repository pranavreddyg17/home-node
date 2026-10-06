//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeGuestChannelDirectoryOwnership(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	parent := filepath.Join(t.TempDir(), "channels")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "guest")
	for i := 0; i < 2; i++ {
		if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); err != nil {
			t.Fatal("ownership transition/retry", err)
		}
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000001, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign owner takeover", err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64056); !errors.Is(err, ErrPolicy) {
		t.Fatal("group drift accepted", err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); err != nil {
		t.Fatal("refusal damaged directory", err)
	}
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), link, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("symlink accepted", err)
	}
	info, err := os.Lstat(target)
	owner, ok := openedSysUID(info)
	if err != nil || !ok || owner != 0 || info.Mode().Perm() != 0700 {
		t.Fatal("target changed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := prepareGuestChannelDirectory(ctx, filepath.Join(parent, "cancelled"), 1000000000, 64055); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "cancelled")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled operation created directory", err)
	}
}
