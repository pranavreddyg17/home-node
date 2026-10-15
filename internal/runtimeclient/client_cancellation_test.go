package runtimeclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalCallPreservesCancellationBeforeHeadersAndDuringBody(t *testing.T) {
	for _, bodyStarted := range []bool{false, true} {
		name := "before-headers"
		if bodyStarted {
			name = "during-body"
		}
		t.Run(name, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				_, _ = io.Copy(io.Discard, r.Body)
				if bodyStarted {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("{"))
					if err := http.NewResponseController(w).Flush(); err != nil {
						return
					}
				}
				close(started)
				<-r.Context().Done()
			}))
			t.Cleanup(server.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport}
			finished := make(chan error, 1)
			go func() {
				var response map[string]any
				finished <- call(ctx, client, "/v1/guest", map[string]string{"fixture": "cancel"}, &response)
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("local request did not reach the fixture")
			}
			cancel()
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation cause lost", err)
				}
				if strings.Contains(err.Error(), server.Listener.Addr().String()) {
					t.Fatal("local address exposed", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("call did not stop on cancellation")
			}
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP peer remained connected after cancellation")
			}
		})
	}
}
