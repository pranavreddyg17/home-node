//go:build linux

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRootEndpointDefaultACLRefusal(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only Linux ACL fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "service.sock")
	parents, err := lockEndpointParents([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer parents[dir].Close()
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Linux POSIX ACL v2: default owner/group/other entries. Unlike an access
	// ACL, this changes new socket permissions without changing parent mode.
	acl := make([]byte, 28)
	binary.LittleEndian.PutUint32(acl, 2)
	for index, tag := range []uint16{0x01, 0x04, 0x20} {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(acl[offset:], tag)
		binary.LittleEndian.PutUint16(acl[offset+2:], 7)
		binary.LittleEndian.PutUint32(acl[offset+4:], ^uint32(0))
	}
	if err := unix.Fsetxattr(int(parents[dir].Fd()), "system.posix_acl_default", acl, 0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("ACL fixture changed directory permissions", err)
	}
	if err := retireStaleEndpoint(parents[dir], path, "unix", 1); err == nil {
		t.Fatal("default ACL drift admitted")
	}
	if other, err := lockEndpointParents([]string{path}); err == nil {
		for _, p := range other {
			p.Close()
		}
		t.Fatal("parent with default ACL admitted")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("refusal created endpoint", err)
	}
}
