//go:build linux

package supervisor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixPeerIdentityIsKernelDerived(t *testing.T) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "peer.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
		if err == nil {
			conn.Close()
		}
		done <- err
	}()
	_ = listener.SetDeadline(time.Now().Add(time.Second))
	conn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	uid, err := PeerUID(conn)
	if err != nil {
		t.Fatal(err)
	}
	if uid != uint32(os.Getuid()) {
		t.Fatalf("peer identity mismatch %d", uid)
	}
	identity, err := PeerProcessIdentity(conn)
	if err != nil || identity.PID != int32(os.Getpid()) || identity.UID != uint32(os.Getuid()) || identity.GID != uint32(os.Getgid()) {
		t.Fatal("kernel process identity mismatch", identity, err)
	}
	if identity, err := PeerProcessIdentity(nil); err == nil || identity != (UnixPeerIdentity{}) {
		t.Fatal("nil peer admitted")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
