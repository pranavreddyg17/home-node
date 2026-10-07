//go:build linux

package hostcheck

import (
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
	for _, path := range []string{regular, link, directory, filepath.Join(directory, "missing"), "relative", "/dev/null"} {
		if probeKVMDeviceAt(path) {
			t.Fatal("non-KVM path reported usable", path)
		}
	}
	data, err := os.ReadFile(regular)
	if err != nil || string(data) != "not a KVM device" {
		t.Fatal("probe changed ordinary file", err)
	}
}
