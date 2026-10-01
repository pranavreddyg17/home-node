//go:build linux

package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDispatchServerCancelsAndJoinsWorker(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("controller peer must be unprivileged")
	}
	for _, scenario := range []string{"cancel", "listener-failure", "worker-failure"} {
		t.Run(scenario, func(t *testing.T) {
			address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "worker.sock")}
			listener, err := net.ListenUnix("unixpacket", address)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan struct{})
			exited := make(chan struct{})
			release := make(chan struct{})
			failure := errors.New("worker publication failure")
			done := make(chan error, 1)
			go func() {
				done <- ServeDispatch(ctx, listener, uid, func(jobContext context.Context, dispatch Dispatch) error {
					defer close(exited)
					close(entered)
					if scenario == "worker-failure" {
						<-release
						return failure
					}
					<-jobContext.Done()
					return jobContext.Err()
				})
			}()
			sender, err := net.DialUnix("unixpacket", nil, address)
			if err != nil {
				t.Fatal(err)
			}
			defer sender.Close()
			dispatch := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
			if err = SendDispatch(ctx, sender, uid, dispatch); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("worker not entered")
			}
			overflow, overflowErr := net.DialUnix("unixpacket", nil, address)
			if overflowErr != nil {
				t.Fatal(overflowErr)
			}
			defer overflow.Close()
			if err = overflow.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 1)
			if n, readErr := overflow.Read(data); n != 0 || readErr == nil {
				t.Fatal("overflow connection not closed")
			} else if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
				t.Fatal("overflow connection retained")
			}
			switch scenario {
			case "cancel":
				cancel()
			case "listener-failure":
				listener.Close()
			case "worker-failure":
				close(release)
			}
			select {
			case err = <-done:
				if scenario == "cancel" && err != nil || scenario == "listener-failure" && err == nil || scenario == "worker-failure" && !errors.Is(err, failure) {
					t.Fatal("server outcome", scenario, err)
				}
				select {
				case <-exited:
				default:
					t.Fatal("server returned before worker exit")
				}
			case <-time.After(time.Second):
				t.Fatal("server failed to join worker")
			}
		})
	}
}
