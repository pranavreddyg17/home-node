package control

import (
	"context"
	"io"
	"net"
	"net/http"
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

func TestMaintenanceServerCancelsAndJoinsActiveRequests(t *testing.T) {
	for _, mode := range []string{"cancel", "listener-error"} {
		t.Run(mode, func(t *testing.T) {
			directory, err := os.MkdirTemp("/tmp", "hn-app-active-")
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
			entered := make(chan struct{})
			exited := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- serveMaintenance(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					close(entered)
					<-r.Context().Done()
					close(exited)
				}))
			}()
			client, err := net.Dial("unix", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err = io.WriteString(client, "POST / HTTP/1.1\r\nHost: local\r\nContent-Length: 0\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request not admitted")
			}
			if mode == "cancel" {
				cancel()
			} else {
				listener.Close()
			}
			select {
			case err := <-done:
				if mode == "cancel" && err != nil {
					t.Fatal(err)
				}
				if mode == "listener-error" && err == nil {
					t.Fatal("listener error hidden")
				}
				select {
				case <-exited:
				default:
					t.Fatal("active handler not joined")
				}
			case <-time.After(time.Second):
				t.Fatal("active request survived service termination")
			}
		})
	}
}
