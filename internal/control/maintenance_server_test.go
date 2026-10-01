package control

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceServerListenerLifetime(t *testing.T) {
	for _, mode := range []string{"cancel", "listener-error"} {
		t.Run(mode, func(t *testing.T) {
			server := testServer(t)
			directory, err := os.MkdirTemp("/tmp", "hn-app-server-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(directory)
			listener, err := net.Listen("unix", filepath.Join(directory, "apps.sock"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- server.ServeMaintenance(ctx, listener, 1001, 1003) }()
			if mode == "cancel" {
				cancel()
			} else {
				listener.Close()
			}
			select {
			case err := <-done:
				if mode == "cancel" && err != nil {
					t.Fatal("normal shutdown failed", err)
				}
				if mode == "listener-error" && err == nil {
					t.Fatal("listener failure hidden")
				}
			case <-time.After(time.Second):
				t.Fatal("maintenance service did not exit")
			}
		})
	}
}
