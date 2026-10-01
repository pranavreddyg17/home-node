//go:build linux

package supervisor

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceDiskServerCancelsHandlersOnListenerExit(t *testing.T) {
	for _, mode := range []string{"cancel", "listener-error"} {
		t.Run(mode, func(t *testing.T) {
			manager, _ := newManager(t)
			address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "disk.sock")}
			listener, err := net.ListenUnix("unixpacket", address)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan struct{}, 2)
			done := make(chan error, 1)
			go func() {
				done <- manager.serveMaintenanceDisks(ctx, listener, 1003, func(ctx context.Context, connection *net.UnixConn, uid uint32) error {
					entered <- struct{}{}
					<-ctx.Done()
					return ctx.Err()
				})
			}()
			clients := []*net.UnixConn{}
			defer func() {
				for _, client := range clients {
					client.Close()
				}
			}()
			for i := 0; i < 2; i++ {
				client, err := net.DialUnix("unixpacket", nil, address)
				if err != nil {
					t.Fatal(err)
				}
				clients = append(clients, client)
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("handler not admitted")
				}
			}
			overflow, err := net.DialUnix("unixpacket", nil, address)
			if err != nil {
				t.Fatal(err)
			}
			defer overflow.Close()
			if err = overflow.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var data [1]byte
			if _, err = overflow.Read(data[:]); err == nil {
				t.Fatal("excess connection retained")
			}
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				t.Fatal("excess connection was not promptly closed")
			}
			if mode == "cancel" {
				cancel()
			} else {
				listener.Close()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("listener exit hidden")
				}
			case <-time.After(time.Second):
				t.Fatal("active handlers survived listener exit")
			}
		})
	}
}
