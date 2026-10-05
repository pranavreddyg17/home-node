package backup

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRecoveryPreparationRefusesBeforeJournalingUnqualifiedSource(t *testing.T) {
	source, manifest, policy, _ := recoverySet(t)
	if disks, err := prepareRecoveryDisks(context.Background(), source, source, strings.Repeat("c", 64), manifest, policy); !errors.Is(err, ErrManifest) || disks != nil {
		t.Fatal("source directory used for disk publication", disks, err)
	}
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if disks, err := prepareRecoveryDisks(context.Background(), source, destination, strings.Repeat("c", 64), manifest, policy); err == nil || disks != nil {
		t.Fatal("unqualified source prepared", disks, err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed qualification produced effects", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if disks, err := prepareRecoveryDisks(ctx, source, destination, strings.Repeat("c", 64), manifest, policy); !errors.Is(err, context.Canceled) || disks != nil {
		t.Fatal("cancelled preparation returned inventory", disks, err)
	}
}
