package main

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

type blockedGuest struct{ started chan struct{} }

func (b blockedGuest) Serve(stream io.ReadWriter) error {
	close(b.started)
	_, err := io.Copy(io.Discard, stream)
	return err
}
func TestRunGuestCancellationClosesActiveRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, peer := net.Pipe()
	defer peer.Close()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runGuest(ctx, blockedGuest{started}, func() (io.ReadWriteCloser, error) { return stream, nil })
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("channel not served")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown stuck in read")
	}
}
func TestRunGuestCancellationInterruptsReconnectWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempted := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- runGuest(ctx, blockedGuest{make(chan struct{})}, func() (io.ReadWriteCloser, error) { attempted <- struct{}{}; return nil, os.ErrNotExist })
	}()
	<-attempted
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown stuck in retry wait")
	}
}
