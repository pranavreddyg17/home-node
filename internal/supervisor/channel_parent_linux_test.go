//go:build linux

package supervisor

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
)

func qualifyReservedChannelParentFixture(t *testing.T, path string, gid int) {
	t.Helper()
	if err := os.Chown(path, 0, gid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0710); err != nil {
		t.Fatal(err)
	}
}

func TestReservedChannelParentRequiresPublishedTransferTraversal(t *testing.T) {
	m := &Manager{GuestGID: 64054, Backend: LinuxBackend{TransferGID: 64055}}
	qualified := unix.Stat_t{Mode: unix.S_IFDIR | 0710, Uid: 0, Gid: 64055}
	if !m.reservedChannelParentAdmitted(qualified) {
		t.Fatal("published parent refused")
	}
	for _, change := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Gid = 0 }, func(s *unix.Stat_t) { s.Mode = unix.S_IFDIR | 0755 }, func(s *unix.Stat_t) { s.Mode = unix.S_IFDIR | 0770 }, func(s *unix.Stat_t) { s.Uid = 1 }, func(s *unix.Stat_t) { s.Gid = 64054 }} {
		candidate := qualified
		change(&candidate)
		if m.reservedChannelParentAdmitted(candidate) {
			t.Fatal("foreign parent admitted", candidate)
		}
	}
}
