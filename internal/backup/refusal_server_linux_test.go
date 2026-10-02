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

func TestRefusalServerClosesCredentialBeforeReplyAndRemainsAvailable(t *testing.T) {
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "worker.sock"), Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	checked := make(chan bool, 1)
	go func() {
		done <- serveDispatch(ctx, listener, 351,
			func(context.Context, *net.UnixConn, uint32) (Launch, *os.File, error) { return launch, credential, nil },
			func(context.Context, Launch, *os.File) error { return ErrLaunchRepositoryAdmission },
			sendLaunchCompletion,
			func(ctx context.Context, connection *net.UnixConn, request Launch, cause error) error {
				_, err := credential.Stat()
				checked <- errors.Is(err, os.ErrClosed)
				return sendLaunchRepositoryRefusal(ctx, connection, request, cause)
			})
	}()
	peer, err := net.DialUnix("unixpacket", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	receipt, end := context.WithTimeout(ctx, time.Second)
	defer end()
	if err := waitLaunchCompletion(receipt, peer, launch); !errors.Is(err, ErrLaunchRepositoryRefusedStopped) {
		t.Fatal(err)
	}
	if !<-checked {
		t.Fatal("refusal sent before credential closure")
	}
	select {
	case err := <-done:
		t.Fatal("qualified refusal stopped service", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server failed to join")
	}
}

func TestRefusalReplyFailureStopsAndJoinsServer(t *testing.T) {
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "worker.sock"), Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	failure := errors.New("fixture refusal send failure")
	go func() {
		done <- serveDispatch(ctx, listener, 351,
			func(context.Context, *net.UnixConn, uint32) (Launch, *os.File, error) { return Launch{}, nil, nil },
			func(context.Context, Launch, *os.File) error { return ErrLaunchRepositoryAdmission },
			sendLaunchCompletion,
			func(context.Context, *net.UnixConn, Launch, error) error { return failure })
	}()
	peer, err := net.DialUnix("unixpacket", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatal("reply failure discarded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("failed refusal server did not stop and join")
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	raw := make([]byte, 256)
	n, err := peer.Read(raw)
	if n != 0 || err == nil {
		t.Fatal("failed reply emitted stop evidence", n, err)
	}
}
