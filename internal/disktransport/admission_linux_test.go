//go:build linux

package disktransport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDiskAdmission(t *testing.T) {
	if os.Getenv("HOMENODE_DISK_TRANSPORT_ROOT_INTEGRATION") != "1" {
		t.Skip("requires disposable root disk fixture")
	}
	if os.Geteuid() != 0 {
		t.Fatal("root fixture requires root")
	}
	path := filepath.Join(t.TempDir(), "disk.raw")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err = writer.Truncate(16 << 20); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	metadata := Metadata{Version: 1, InstanceID: strings.Repeat("a", 24), Workload: "files", Bytes: 16 << 20, ImageSHA256: strings.Repeat("a", 64), DataSchema: 1, Protocol: 1}
	if err = AdmitDisk(file, metadata); err != nil {
		t.Fatal("owned disk refused", err)
	}
	if err = AdmitDisk(writer, metadata); err == nil {
		t.Fatal("writable descriptor accepted")
	}
	wrong := metadata
	wrong.Bytes++
	if err = AdmitDisk(file, wrong); err == nil {
		t.Fatal("wrong declared bytes accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err = AdmitDisk(file, metadata); err == nil {
		t.Fatal("unsafe mode accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(path, path+".alias"); err != nil {
		t.Fatal(err)
	}
	if err = AdmitDisk(file, metadata); err == nil {
		t.Fatal("multiply linked disk accepted")
	}
	if err = os.Remove(path + ".alias"); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(path, 1001, 0); err != nil {
		t.Fatal(err)
	}
	if err = AdmitDisk(file, metadata); err == nil {
		t.Fatal("foreign ownership accepted")
	}
}
