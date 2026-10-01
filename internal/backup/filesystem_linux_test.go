//go:build linux

package backup

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestNativeExt4Qualification(t *testing.T) {
	if os.Getenv("HOMENODE_FILESYSTEM_INTEGRATION") != "1" {
		t.Skip("requires disposable Linux e2fsprogs fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	path := filepath.Join(t.TempDir(), "source.raw")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err = writer.Truncate(16 << 20); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "/usr/sbin/mkfs.ext4", "-q", "-F", "-m", "0", "-E", "nodiscard,lazy_itable_init=0,lazy_journal_init=0", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("format fixture: %v: %s", err, output)
	}
	source, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	digest := func() [32]byte {
		t.Helper()
		hash := sha256.New()
		if _, err := io.Copy(hash, io.NewSectionReader(source, 0, 16<<20)); err != nil {
			t.Fatal(err)
		}
		var value [32]byte
		copy(value[:], hash.Sum(nil))
		return value
	}
	before := digest()
	bridge := &qualifiedDiskFixture{file: source}
	invoked := false
	if err = (QualifiedMaintenanceDisks{Source: bridge}).WithMaintenanceDisk(ctx, "owned-token", "owned-instance", func(ctx context.Context, file *os.File, instance supervisor.Instance) error {
		invoked = true
		if !bridge.active || file != source {
			t.Fatal("source lease not retained across copy")
		}
		return nil
	}); err != nil || !invoked || bridge.active {
		t.Fatal("qualified copy failed", err)
	}
	if err = QualifyExt4Disk(ctx, source); err != nil {
		t.Fatal("clean filesystem refused", err)
	}
	if digest() != before {
		t.Fatal("checker changed source")
	}
	if err = QualifyExt4Disk(ctx, writer); err == nil {
		t.Fatal("writable source accepted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = QualifyExt4Disk(cancelled, source); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled check accepted", err)
	}
	// Replacement of the path must not redirect descriptor-based checking.
	if err = os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = QualifyExt4Disk(ctx, source); err != nil || digest() != before {
		t.Fatal("checker followed replacement", err)
	}
	// Mutate the primary state on the pinned inode; never repair it here.
	if _, err = writer.WriteAt([]byte{0, 0}, 1024+0x3a); err != nil {
		t.Fatal(err)
	}
	dirty := digest()
	invoked = false
	if err = (QualifiedMaintenanceDisks{Source: bridge}).WithMaintenanceDisk(ctx, "owned-token", "owned-instance", func(context.Context, *os.File, supervisor.Instance) error { invoked = true; return nil }); err == nil || invoked || bridge.active {
		t.Fatal("dirty copy admitted", err)
	}
	if err = QualifyExt4Disk(ctx, source); err == nil {
		t.Fatal("dirty filesystem accepted")
	}
	if digest() != dirty {
		t.Fatal("dirty filesystem repaired")
	}
}

type qualifiedDiskFixture struct {
	file   *os.File
	active bool
}

func (d *qualifiedDiskFixture) WithMaintenanceDisk(ctx context.Context, token, id string, copyDisk func(context.Context, *os.File, supervisor.Instance) error) error {
	d.active = true
	defer func() { d.active = false }()
	return copyDisk(ctx, d.file, supervisor.Instance{ID: id, Workload: "files", State: "stopped", Desired: "stopped"})
}
