//go:build linux

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeFreshVolumeFormattingPreservesExistingData(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	parent := t.TempDir()
	path := filepath.Join(parent, "fresh.raw")
	const size = 64 << 20
	if err := prepareDataVolume(context.Background(), path, size, 4<<30); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var magic [2]byte
	_, err = file.ReadAt(magic[:], 1024+56)
	file.Close()
	if err != nil || magic != [2]byte{0x53, 0xef} {
		t.Fatal("new volume missing ext4 superblock magic")
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != size || info.Mode().Perm() != 0600 {
		t.Fatal("new volume metadata mismatch")
	}
	existing := filepath.Join(parent, "existing.raw")
	out, err := os.OpenFile(existing, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := out.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteAt([]byte("existing owner data"), 0); err != nil {
		t.Fatal(err)
	}
	out.Close()
	if err := prepareDataVolume(context.Background(), existing, size, 4<<30); err != nil {
		t.Fatal(err)
	}
	out, err = os.Open(existing)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 19)
	_, err = out.ReadAt(data, 0)
	out.Close()
	if err != nil || string(data) != "existing owner data" {
		t.Fatal("existing volume reformatted")
	}
	link := filepath.Join(parent, "symlink.raw")
	if err := os.Symlink(existing, link); err != nil {
		t.Fatal(err)
	}
	if prepareDataVolume(context.Background(), link, size, 4<<30) == nil {
		t.Fatal("symlink volume admitted")
	}
	alias := filepath.Join(parent, "alias.raw")
	if err := os.Link(existing, alias); err != nil {
		t.Fatal(err)
	}
	if prepareDataVolume(context.Background(), existing, size, 4<<30) == nil {
		t.Fatal("hardlink alias volume admitted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if prepareDataVolume(cancelled, filepath.Join(parent, "cancelled.raw"), size, 4<<30) == nil {
		t.Fatal("cancelled initialization accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "cancelled.raw")); !os.IsNotExist(err) {
		t.Fatal("cancelled initialization published data")
	}
}
