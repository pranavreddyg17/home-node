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
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
