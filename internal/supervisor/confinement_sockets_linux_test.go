//go:build linux

package supervisor

import (
	"testing"

	"golang.org/x/sys/unix"
)

// Disposable fixture calls the shared observer without granting its candidate
// role metadata production authorization.
func nativeQEMUSocketInventory(t *testing.T, pidfd, number int, expected unix.Stat_t) qemuSocketMetadata {
	t.Helper()
	metadata, err := observeQEMUSocketMetadata(pidfd, number, expected)
	if err != nil {
		t.Fatal("socket metadata observation refused", err)
	}
	return metadata
}
