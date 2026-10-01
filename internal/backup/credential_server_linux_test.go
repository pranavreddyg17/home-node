//go:build linux

package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCredentialServerJoinsWorkAndClosesReceivedDescriptor(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("controller peer must be unprivileged")
	}
	for _, scenario := range []string{"cancel", "worker-failure", "listener-failure"} {
		t.Run(scenario, func(t *testing.T) {
			address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "credential.sock")}
			listener, err := net.ListenUnix("unixpacket", address)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan *os.File, 1)
			exited := make(chan struct{})
			release := make(chan struct{})
			failure := errors.New("credential worker failure")
			done := make(chan error, 1)
			var calls atomic.Int32
			go func() {
				done <- ServeCredentialDispatch(ctx, listener, uid, func(operation context.Context, job Dispatch, credential *os.File) error {
					calls.Add(1)
					defer close(exited)
					entered <- credential
					if scenario == "worker-failure" {
						<-release
						return failure
					}
					<-operation.Done()
					return operation.Err()
				})
			}()
			sender, err := net.DialUnix("unixpacket", nil, address)
			if err != nil {
				t.Fatal(err)
			}
			defer sender.Close()
			source, err := CreateRepositoryPassword([]byte("fixture-only-secret"))
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			job := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
			if err = SendCredentialDispatch(ctx, sender, uid, job, source); err != nil {
				t.Fatal(err)
			}
			var received *os.File
			select {
			case received = <-entered:
			case <-time.After(time.Second):
				t.Fatal("credential worker not entered")
			}
			overflow, err := net.DialUnix("unixpacket", nil, address)
			if err != nil {
				t.Fatal(err)
			}
			defer overflow.Close()
			// Admission may close before send; either write outcome must leave
			// this connection closed and the source credential retained.
			_ = SendCredentialDispatch(ctx, overflow, uid, job, source)
			if err = overflow.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 1)
			n, readErr := overflow.Read(data)
			if n > 0 || readErr == nil {
				t.Fatal("overflow credential connection returned data")
			}
			if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
				t.Fatal("overflow credential connection retained")
			}
			if calls.Load() != 1 {
				t.Fatal("overflow credential entered worker")
			}
			switch scenario {
			case "cancel":
				cancel()
			case "worker-failure":
				close(release)
			case "listener-failure":
				listener.Close()
			}
			select {
			case err = <-done:
				if scenario == "cancel" && err != nil || scenario == "worker-failure" && !errors.Is(err, failure) || scenario == "listener-failure" && err == nil {
					t.Fatal("credential server outcome", err)
				}
				select {
				case <-exited:
				default:
					t.Fatal("worker not joined")
				}
				if _, err = received.Stat(); err == nil {
					t.Fatal("received credential retained after shutdown")
				}
				if _, err = source.Stat(); err != nil {
					t.Fatal("server closed sender credential", err)
				}
			case <-time.After(time.Second):
				t.Fatal("credential server shutdown stuck")
			}
		})
	}
}
