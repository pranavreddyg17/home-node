package networkcheck

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestListenerHealthFailureUnblocksAccept(t *testing.T) {
	raw, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var healthy atomic.Bool
	healthy.Store(true)
	listener := watchListener(raw, time.Millisecond, healthy.Load)
	defer listener.Close()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := listener.Accept()
	client.Close()
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	result := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if connection != nil {
			connection.Close()
		}
		result <- err
	}()
	healthy.Store(false)
	select {
	case err := <-result:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("health failure did not close listener: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("health failure left accept blocked")
	}
	select {
	case <-listener.done:
	case <-time.After(time.Second):
		t.Fatal("health watcher did not exit")
	}
}

func TestListenerCloseStopsHealthWatcher(t *testing.T) {
	raw, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := watchListener(raw, time.Hour, func() bool { return true })
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-listener.done:
	case <-time.After(time.Second):
		t.Fatal("normal close leaked the health watcher")
	}
}
