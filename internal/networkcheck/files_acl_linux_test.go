package networkcheck

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTLSDetectsRestoredMetadataAfterMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate equal-length replacement followed by restoring the visible
	// modification timestamp. Linux change time must still invalidate it.
	time.Sleep(time.Millisecond)
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("fixture did not restore visible metadata")
	}
	if tlsFileUnchanged(before, after) {
		t.Fatal("mutated file admitted with restored timestamp")
	}
	if !tlsFileUnchanged(after, after) {
		t.Fatal("unchanged observation refused")
	}
}

func TestRootTLSRejectsNamedReaderACL(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires disposable root-owned Linux fixture")
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("fixture key"), 0640); err != nil {
		t.Fatal(err)
	}
	// Linux POSIX ACL v2: owner rw, named uid 12345 r, owning group
	// none, mask r, others none. The visible mode remains 0640.
	acl := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(acl, 2)
	for i, entry := range []struct {
		tag, perm uint16
		id        uint32
	}{
		{1, 6, ^uint32(0)}, {2, 4, 12345}, {4, 0, ^uint32(0)},
		{16, 4, ^uint32(0)}, {32, 0, ^uint32(0)},
	} {
		offset := 4 + i*8
		binary.LittleEndian.PutUint16(acl[offset:], entry.tag)
		binary.LittleEndian.PutUint16(acl[offset+2:], entry.perm)
		binary.LittleEndian.PutUint32(acl[offset+4:], entry.id)
	}
	if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("fixture did not preserve expected mode", err)
	}
	if _, err := protectedPEM(path, true); err == nil {
		t.Fatal("named ACL reader admitted")
	}
}
