//go:build linux

package hostcheck

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestKVMProbeRefusesOrdinaryAndUnrelatedDevices(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "kvm")
	if err := os.WriteFile(regular, []byte("not a KVM device"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "alias")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(directory, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{regular, link, fifo, directory, filepath.Join(directory, "missing"), "relative", "/dev/null"} {
		if probeKVMDeviceAt(path) {
			t.Fatal("non-KVM path reported usable", path)
		}
	}
	data, err := os.ReadFile(regular)
	if err != nil || string(data) != "not a KVM device" {
		t.Fatal("probe changed ordinary file", err)
	}
}

func TestNativeKVMAPIAccess(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_API_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux KVM fixture")
	}
	if !probeKVMDevice() {
		t.Fatal("supported KVM device/API could not be qualified")
	}
}
