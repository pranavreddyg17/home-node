//go:build linux

package main

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The same-UID temporary fixture qualifies unprivileged socket creation and
// refusal behavior. Installed distinct groups still require a root fixture.
func TestPrivateControllerListenerPermissionsAndExistingEndpointRefusal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unprivileged Linux socket fixture")
	}
	socket := filepath.Join(t.TempDir(), "control.sock")
	listener, err := listenControllerProxy(socket, uint32(os.Getegid()))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Lstat(socket)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0660 || owner.Uid != uint32(os.Geteuid()) || owner.Gid != uint32(os.Getegid()) {
		t.Fatal("private socket identity differs", info)
	}
	if other, err := listenControllerProxy(socket, uint32(os.Getegid())); err == nil {
		other.Close()
		t.Fatal("live endpoint replaced")
	}
	after, err := os.Lstat(socket)
	if err != nil || !os.SameFile(info, after) {
		t.Fatal("existing endpoint changed", err)
	}
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal("original listener lost", err)
	}
	connection.Close()
	listener.Close()
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatal("clean close retained endpoint", err)
	}
	if err := os.WriteFile(socket, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := listenControllerProxy(socket, uint32(os.Getegid())); err == nil {
		other.Close()
		t.Fatal("foreign file replaced")
	}
	data, err := os.ReadFile(socket)
	if err != nil || string(data) != "retain" {
		t.Fatal("foreign file modified", err)
	}
}

func TestPrivateControllerListenerRefusesUnsafeParents(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0777); err != nil {
		t.Fatal(err)
	}
	if listener, err := listenControllerProxy(filepath.Join(directory, "control.sock"), uint32(os.Getegid())); err == nil {
		listener.Close()
		t.Fatal("writable parent accepted")
	}
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "parent")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if listener, err := listenControllerProxy(filepath.Join(link, "control.sock"), uint32(os.Getegid())); err == nil {
		listener.Close()
		t.Fatal("symlink parent accepted")
	}
}
