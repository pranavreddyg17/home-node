package networkcheck

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type gatedAcceptListener struct {
	net.Listener
	accepted, release chan struct{}
}

func (listener *gatedAcceptListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	close(listener.accepted)
	<-listener.release
	return connection, err
}

func TestInterfaceFailureDiscardsConcurrentlyAcceptedConnection(t *testing.T) {
	raw, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gated := &gatedAcceptListener{Listener: raw, accepted: make(chan struct{}), release: make(chan struct{})}
	var healthy atomic.Bool
	healthy.Store(true)
	listener := watchListener(gated, time.Millisecond, healthy.Load)
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if connection != nil {
			connection.Close()
			err = errors.New("invalid interface delivered a connection")
		}
		result <- err
	}()
	defer close(gated.release)
	client, err := net.DialTimeout("tcp4", raw.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-gated.accepted:
	case <-time.After(time.Second):
		t.Fatal("underlying listener did not accept")
	}
	healthy.Store(false)
	select {
	case <-listener.done:
	case <-time.After(time.Second):
		t.Fatal("interface health failure did not close listener")
	}
	// Release the already accepted connection only after the failed health
	// observation; the wrapper must discard it instead of handing it to TLS.
	gated.release <- struct{}{}
	select {
	case err := <-result:
		if !errors.Is(err, ErrIdentity) {
			t.Fatalf("concurrent acceptance did not fail identity admission: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent acceptance remained blocked")
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("discarded connection remained usable")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("discarded connection remained open")
	}
}

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
		if !errors.Is(err, net.ErrClosed) || !errors.Is(err, ErrIdentity) {
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
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) || errors.Is(err, ErrIdentity) {
		t.Fatalf("normal close incorrectly reported an ingress identity failure: %v", err)
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
