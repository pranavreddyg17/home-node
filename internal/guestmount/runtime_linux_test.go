//go:build linux

package guestmount

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
)

func TestRuntimeFilesystemLimits(t *testing.T) {
	good := unix.Statfs_t{Type: unix.TMPFS_MAGIC, Flags: unix.ST_NOSUID | unix.ST_NODEV | unix.ST_NOEXEC, Bsize: 4096, Blocks: 16384, Files: 8192}
	if admitRuntimeFS(good) != nil {
		t.Fatal("valid filesystem refused")
	}
	changes := []func(*unix.Statfs_t){
		func(s *unix.Statfs_t) { s.Type = unix.EXT4_SUPER_MAGIC },
		func(s *unix.Statfs_t) { s.Flags &^= unix.ST_NOEXEC },
		func(s *unix.Statfs_t) { s.Flags &^= unix.ST_NODEV },
		func(s *unix.Statfs_t) { s.Flags &^= unix.ST_NOSUID },
		func(s *unix.Statfs_t) { s.Bsize = 0 },
		func(s *unix.Statfs_t) { s.Bsize = -1 },
		func(s *unix.Statfs_t) { s.Blocks++ },
		func(s *unix.Statfs_t) { s.Blocks = 0 },
		func(s *unix.Statfs_t) { s.Files++ },
		func(s *unix.Statfs_t) { s.Files = 0 },
	}
	for i, change := range changes {
		bad := good
		change(&bad)
		if admitRuntimeFS(bad) == nil {
			t.Fatalf("unsafe filesystem %d accepted", i)
		}
	}
}

func TestNativeRuntimeMountAdmission(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_RUNTIME_INTEGRATION") != "1" {
		t.Skip("requires disposable private mount namespace")
	}
	parent := os.Getenv("HOMENODE_GUEST_RUNTIME_PARENT_NAMESPACE")
	current, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil || parent == "" || parent == current {
		t.Fatal("fixture is not in a new mount namespace")
	}
	expectation := os.Getenv("HOMENODE_GUEST_RUNTIME_EXPECT")
	err = CheckRuntime()
	switch expectation {
	case "accept":
		if err != nil {
			t.Fatal(err)
		}
	case "reject":
		if err == nil {
			t.Fatal("unsafe runtime mounts accepted")
		}
	default:
		t.Fatal("missing fixture expectation")
	}
}
