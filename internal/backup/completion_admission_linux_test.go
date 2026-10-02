//go:build linux

package backup

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompletionAdmissionWaitsUntilReplySlotReleased(t *testing.T) {
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "worker.sock"), Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- serveDispatch(ctx, listener, 351,
			func(context.Context, *net.UnixConn, uint32) (int, *os.File, error) { return 1, nil, nil },
			func(context.Context, int, *os.File) error {
				if count.Add(1) == 2 {
					close(second)
				}
				return nil
			},
			func(operation context.Context, _ *net.UnixConn, _ int) error {
				if count.Load() == 1 {
					close(entered)
					select {
					case <-release:
					case <-operation.Done():
						return operation.Err()
					}
				}
				return nil
			})
	}()
	first, err := net.DialUnix("unixpacket", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("completion did not start")
	}
	next, err := net.DialUnix("unixpacket", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	// The next connection must remain queued, rather than being closed by the
	// still-held callback slot after the preceding reply became visible.
	next.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = next.Read(make([]byte, 1))
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal("next operation rejected during completion", err)
	}
	next.SetReadDeadline(time.Time{})
	close(release)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("queued operation not admitted after completion")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not join shutdown")
	}
}
