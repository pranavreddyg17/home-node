package backup

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRecoveryDiskCopyIntegrityAndOccupiedTarget(t *testing.T) {
	source, manifest, _, _ := recoverySet(t)
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	entry := manifest.Files[1]
	disk := RecoveryInstallDisk{Workload: entry.Workload, SourceName: entry.Name, Bytes: entry.Bytes, SourceSHA256: entry.SHA256, InstanceID: state.Random()}
	target := ".recovery-" + disk.InstanceID + ".stage"
	if err = copyRecoveryDisk(context.Background(), source, destination, disk, target); err != nil {
		t.Fatal(err)
	}
	expected, _ := source.ReadFile(entry.Name)
	actual, err := destination.ReadFile(target)
	if err != nil || string(actual) != string(expected) {
		t.Fatal("copy differs", err)
	}
	if err = copyRecoveryDisk(context.Background(), source, destination, disk, target); !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied target adopted", err)
	}
	bad := disk
	bad.InstanceID = state.Random()
	bad.SourceSHA256 = strings.Repeat("0", 64)
	badTarget := ".recovery-" + bad.InstanceID + ".stage"
	if err = copyRecoveryDisk(context.Background(), source, destination, bad, badTarget); !errors.Is(err, ErrManifest) {
		t.Fatal("wrong digest accepted", err)
	}
	if _, err = destination.Lstat(badTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed copy retained", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = copyRecoveryDisk(ctx, source, destination, disk, target); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
