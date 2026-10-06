//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
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
	socketPath := filepath.Join(path, "adapter.sock")
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: socketPath}); err != nil {
		unix.Close(fd)
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(socketPath, 1000000000, 64055); err != nil {
		t.Fatal(err)
	}
	if err := grantGuestChannelAccess(context.Background(), socketPath, 1000000000, 64055); err != nil {
		t.Fatal("pinned socket access grant", err)
	}
	granted, err := os.Lstat(socketPath)
	if err != nil || granted.Mode().Perm() != 0660 {
		t.Fatal("socket grant mode", err)
	}
	if err := grantGuestChannelAccess(context.Background(), socketPath, 1000000001, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("wrong socket identity granted", err)
	}
	socketBefore, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); err != nil {
		t.Fatal("existing guest socket refused", err)
	}
	socketAfter, err := os.Lstat(socketPath)
	if err != nil || !os.SameFile(socketBefore, socketAfter) || socketBefore.Mode() != socketAfter.Mode() {
		t.Fatal("retry changed socket", err)
	}
	if err := os.Chown(socketPath, 1000000001, 64055); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign socket accepted", err)
	}
	if err := os.Chown(socketPath, 1000000000, 64055); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias.sock")
	if err := os.Link(socketPath, alias); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("aliased socket accepted", err)
	}
	if err := grantGuestChannelAccess(context.Background(), socketPath, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("aliased socket grant", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); err != nil {
		t.Fatal("socket retry after refusal", err)
	}
	unknown := filepath.Join(path, "unknown")
	if err := os.WriteFile(unknown, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), path, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("unknown guest-directory entry accepted", err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "retained" {
		t.Fatal("unknown entry changed", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
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
	occupied := filepath.Join(parent, "occupied")
	if err := os.Mkdir(occupied, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(occupied, "secret")
	if err := os.WriteFile(sentinel, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareGuestChannelDirectory(context.Background(), occupied, 1000000000, 64055); !errors.Is(err, ErrPolicy) {
		t.Fatal("occupied root directory transferred", err)
	}
	info, err = os.Lstat(occupied)
	owner, ok = openedSysUID(info)
	if err != nil || !ok || owner != 0 || info.Mode().Perm() != 0700 {
		t.Fatal("refused directory changed", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "retained" {
		t.Fatal("sentinel changed", err)
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
