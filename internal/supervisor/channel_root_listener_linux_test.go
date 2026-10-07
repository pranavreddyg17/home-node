//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNativeGuestChannelRootListenerRefusal(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "guest")
	const guestUID = 2000000000
	const transferGID = 64055
	if err := prepareGuestChannelDirectory(context.Background(), directory, guestUID, transferGID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "adapter.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	if err := os.Chmod(path, 0775); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := grantGuestChannelAccess(context.Background(), path, guestUID, transferGID); err == nil {
		t.Fatal("generic helper adopted root socket")
	}
	expected := UnixPeerIdentity{PID: int32(os.Getpid()), UID: guestUID, GID: 993}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := grantLibvirtGuestChannelAccess(cancelled, path, guestUID, transferGID, expected); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled socket adoption did not preserve cancellation", err)
	}
	if err := grantLibvirtGuestChannelAccess(context.Background(), path, guestUID, transferGID, expected); err == nil {
		t.Fatal("root peer admitted as guest")
	}
	// Require the peer probe to have actually connected and closed without
	// an adapter payload, rather than passing through an earlier refusal.
	if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	accepted, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal("root peer probe was not exercised", err)
	}
	defer accepted.Close()
	if err := accepted.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if n, err := accepted.Read(buffer); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatal("denied root peer received payload", n, err)
	}
	alias := filepath.Join(directory, "alias.sock")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := grantLibvirtGuestChannelAccess(context.Background(), path, guestUID, transferGID, expected); err == nil {
		t.Fatal("aliased root listener admitted")
	}
	if err := listener.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	unexpected, err := listener.AcceptUnix()
	if unexpected != nil {
		unexpected.Close()
		t.Fatal("aliased listener reached peer probe")
	}
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatal("alias refusal probe observation ambiguous", err)
	}
	aliased, err := os.Lstat(alias)
	if err != nil || !os.SameFile(before, aliased) || aliased.Mode() != before.Mode() {
		t.Fatal("alias refusal changed socket", err)
	}
	aliasMetadata, ok := aliased.Sys().(*syscall.Stat_t)
	if !ok || aliasMetadata.Uid != 0 || aliasMetadata.Gid != 0 || aliasMetadata.Nlink != 2 {
		t.Fatal("alias refusal changed ownership")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := after.Sys().(*syscall.Stat_t)
	if !ok || !os.SameFile(before, after) || after.Mode() != before.Mode() || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Nlink != 1 {
		t.Fatal("refusal changed root listener")
	}
}
