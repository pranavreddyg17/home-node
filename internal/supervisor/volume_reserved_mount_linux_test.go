//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestNativeReservedVolumeMountRefusal(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root mount fixture")
	}
	t.Run("ChannelSocket", testChannelSocketMountRefusal)
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
	m, _ := newManager(t)
	pool := GuestUIDPool{First: d.GuestUID, Last: d.GuestUID + 1}
	m.GuestUIDPool, m.GuestGID = &pool, d.GuestGID
	d.GuestUID, d.GuestGID = 0, 0
	d.Image.SHA256 = strings.Repeat("a", 64)
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, d.Image.DataBytes); err != nil {
		t.Fatal(err)
	}
	provenance, err := openReservedVolume(ctx, directory, d)
	if err != nil {
		t.Fatal(err)
	}
	_, recordErr := m.recordPinnedVolumeOwnership(ctx, d, provenance)
	if err := errors.Join(recordErr, provenance.Close()); err != nil {
		t.Fatal(err)
	}
	if err := m.withReservedVolume(ctx, directory, d, func(ctx context.Context) error { return ctx.Err() }, func(ctx context.Context, file *os.File, intent VolumeOwnershipIntent, guard func(context.Context) error) error {
		if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
			return err
		}
		mounted = true
		var sameInode unix.Stat_t
		if unix.Stat(target, &sameInode) != nil || sameInode.Dev != original.Dev || sameInode.Ino != original.Ino {
			t.Error("self-bind fixture changed inode")
		}
		guardErr := guard(ctx)
		if err := unix.Unmount(target, 0); err != nil {
			return errors.Join(guardErr, err)
		}
		mounted = false
		if !errors.Is(guardErr, ErrPolicy) {
			t.Fatal("retained scope accepted same-inode bind mount", guardErr)
		}
		return guardErr
	}); !errors.Is(err, ErrPolicy) || mounted {
		t.Fatal("retained scope mount refusal", mounted, err)
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
	retained, err := unix.Open(target, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(retained); err != nil {
			t.Error(err)
		}
	})
	if !samePathMount(retained, unix.AT_FDCWD, target) {
		t.Fatal("ordinary path mount mismatch")
	}
	if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
		t.Fatal("self bind mount unavailable", err)
	}
	mounted = true
	var selfBound unix.Stat_t
	if unix.Stat(target, &selfBound) != nil || selfBound.Dev != original.Dev || selfBound.Ino != original.Ino {
		t.Fatal("self bind fixture changed inode identity")
	}
	if samePathMount(retained, unix.AT_FDCWD, target) {
		t.Fatal("self-bind admitted by cgroup path mount recheck")
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
	// Maintenance must qualify the same mount boundary with read-only access
	// after ownership has moved to the independently reserved guest identity.
	if err := os.Chown(target, int(d.GuestUID), int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	readOnly, err := openReservedMaintenanceVolume(ctx, directory, d)
	if err != nil {
		t.Fatal("ordinary maintenance disk refused", err)
	}
	if _, err := readOnly.WriteAt([]byte("forbidden"), 0); err == nil {
		t.Fatal("maintenance descriptor writable")
	}
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	mounted = true
	readOnly, err = openReservedMaintenanceVolume(ctx, directory, d)
	if !errors.Is(err, ErrPolicy) || readOnly != nil {
		if readOnly != nil {
			_ = readOnly.Close()
		}
		t.Fatal("maintenance admitted self-bind", err)
	}
	if err := unix.Unmount(target, 0); err != nil {
		t.Fatal(err)
	}
	mounted = false
	if err := os.Chown(target, 0, 0); err != nil {
		t.Fatal(err)
	}
}
