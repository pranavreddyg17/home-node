//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeGuestUIDVolumeAdmission(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	path := filepath.Join(t.TempDir(), "guest.raw")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const size = 16 << 20
	if err = file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err = transferVolumeToGuest(context.Background(), file, size, 200000, 200000); err != nil {
		t.Fatal(err)
	}
	if err = transferVolumeToGuest(context.Background(), file, size, 200000, 200000); err != nil {
		t.Fatal("ownership retry refused", err)
	}
	if err = transferVolumeToGuest(context.Background(), file, size, 200001, 200001); !errors.Is(err, ErrPolicy) {
		t.Fatal("another guest took ownership", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = transferVolumeToGuest(ctx, file, size, 200000, 200000); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled ownership ignored", err)
	}
	if err = admitVolumeForUID(file, size, 200000); err != nil {
		t.Fatal("reserved guest ownership refused", err)
	}
	if err = admitVolume(file, size); !errors.Is(err, ErrPolicy) {
		t.Fatal("guest volume admitted as root-owned", err)
	}
	if err = admitVolumeForUID(file, size, 200001); !errors.Is(err, ErrPolicy) {
		t.Fatal("other guest ownership admitted", err)
	}
	alias := path + ".alias"
	if err = os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); !errors.Is(err, ErrPolicy) {
		t.Fatal("aliased guest volume admitted", err)
	}
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err = file.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); !errors.Is(err, ErrPolicy) {
		t.Fatal("permissive guest volume admitted", err)
	}
	if err = file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []uint32{1, 65535, 1 << 31} {
		if err = admitVolumeForUID(file, size, uid); !errors.Is(err, ErrPolicy) {
			t.Fatal("unreserved UID accepted", uid, err)
		}
	}
}
