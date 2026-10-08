//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestNativeReservedVolumeMountRefusal(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root mount fixture")
	}
	directory, err := os.MkdirTemp("", "homenode-volume-mount-")
	if err != nil {
		t.Fatal(err)
	}
	mounted := false
	t.Cleanup(func() {
		if mounted {
			t.Error("uncertain mount retained with fixture directory", directory)
			return
		}
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 200000}
	d.Image.DataBytes = 16 << 20
	if err := os.Chown(directory, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, d.ID+".raw")
	source := filepath.Join(directory, "source.raw")
	for _, path := range []string{target, source} {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		err = file.Truncate(d.Image.DataBytes)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	var original, other unix.Stat_t
	if unix.Stat(target, &original) != nil || unix.Stat(source, &other) != nil || original.Dev != other.Dev || original.Ino == other.Ino {
		t.Fatal("fixture must use distinct same-filesystem disks")
	}
	ctx := context.Background()
	accepted, err := openReservedVolume(ctx, directory, d)
	if err != nil {
		t.Fatal("ordinary volume refused", err)
	}
	if err := accepted.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(source, target, "", unix.MS_BIND, ""); err != nil {
		t.Fatal("native bind mount unavailable", err)
	}
	mounted = true
	defer func() {
		if mounted {
			if err := unix.Unmount(target, 0); err != nil {
				t.Error("bind mount cleanup failed", err)
			} else {
				mounted = false
			}
		}
	}()
	var bound unix.Stat_t
	if unix.Stat(target, &bound) != nil || bound.Dev != original.Dev || bound.Ino != other.Ino {
		t.Fatal("bind fixture did not preserve filesystem device")
	}
	rejected, err := openReservedVolume(ctx, directory, d)
	if !errors.Is(err, ErrPolicy) || rejected != nil {
		if rejected != nil {
			rejected.Close()
		}
		t.Fatal("same-filesystem bind mount admitted", err)
	}
	if err := unix.Unmount(target, 0); err != nil {
		t.Fatal(err)
	}
	mounted = false
	var restored unix.Stat_t
	if unix.Stat(target, &restored) != nil || restored.Dev != original.Dev || restored.Ino != original.Ino || restored.Mode != original.Mode || restored.Uid != original.Uid || restored.Gid != original.Gid || restored.Size != original.Size {
		t.Fatal("mount refusal changed original disk")
	}
	if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
		t.Fatal("self bind mount unavailable", err)
	}
	mounted = true
	var selfBound unix.Stat_t
	if unix.Stat(target, &selfBound) != nil || selfBound.Dev != original.Dev || selfBound.Ino != original.Ino {
		t.Fatal("self bind fixture changed inode identity")
	}
	rejected, err = openReservedVolume(ctx, directory, d)
	if !errors.Is(err, ErrPolicy) || rejected != nil {
		if rejected != nil {
			rejected.Close()
		}
		t.Fatal("same-inode bind mount admitted", err)
	}
	if err := unix.Unmount(target, 0); err != nil {
		t.Fatal(err)
	}
	mounted = false
	accepted, err = openReservedVolume(ctx, directory, d)
	if err != nil {
		t.Fatal("restored volume refused", err)
	}
	if err := accepted.Close(); err != nil {
		t.Fatal(err)
	}
}
