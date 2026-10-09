//go:build linux

package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func testChannelSocketMountRefusal(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "guest")
	const uid, gid = 200000, 64055
	ctx := context.Background()
	if err := prepareGuestChannelDirectory(ctx, directory, uid, gid); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "adapter.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	if err := os.Chown(path, uid, gid); err != nil {
		t.Fatal(err)
	}
	if err := grantGuestChannelAccess(ctx, path, uid, gid); err != nil {
		t.Fatal("ordinary socket admission", err)
	}
	var original unix.Stat_t
	if unix.Lstat(path, &original) != nil {
		t.Fatal("socket metadata unavailable")
	}
	if err := unix.Mount(path, path, "", unix.MS_BIND, ""); err != nil {
		t.Fatal("socket bind mount fixture", err)
	}
	defer func() {
		if err := unix.Unmount(path, 0); err != nil {
			t.Error("socket fixture unmount", err)
		}
	}()
	var bound unix.Stat_t
	if unix.Lstat(path, &bound) != nil || bound.Dev != original.Dev || bound.Ino != original.Ino {
		t.Fatal("self-bind changed socket inode")
	}
	if err := grantGuestChannelAccess(ctx, path, uid, gid); !errors.Is(err, ErrPolicy) {
		t.Fatal("same-inode mounted socket admitted", err)
	}
	var refused unix.Stat_t
	if unix.Lstat(path, &refused) != nil || refused.Mode != original.Mode || refused.Uid != original.Uid || refused.Gid != original.Gid {
		t.Fatal("mount refusal changed socket metadata")
	}
}
