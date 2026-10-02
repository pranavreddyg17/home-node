//go:build linux || darwin

package backup

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestStagingSpaceUsesOpenedFilesystemAndRejectsMissingDescriptor(t *testing.T) {
	if err := requireStagingSpace(nil, 0); !errors.Is(err, ErrStagingCapacity) {
		t.Fatal("missing staging descriptor admitted", err)
	}
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var space unix.Statfs_t
	if err := unix.Fstatfs(int(directory.Fd()), &space); err != nil {
		directory.Close()
		t.Fatal(err)
	}
	expected := stagingCapacity(space.Bavail, int64(space.Bsize), 0)
	actual := requireStagingSpace(directory, 0)
	if !errors.Is(actual, expected) {
		directory.Close()
		t.Fatal("filesystem capacity mismatch", actual, expected)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	if err := requireStagingSpace(directory, 0); !errors.Is(err, ErrStagingCapacity) {
		t.Fatal("closed staging descriptor admitted", err)
	}
}
