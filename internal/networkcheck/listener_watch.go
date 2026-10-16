package networkcheck

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// A changed interface must cause the installed gateway to exit and let its
// service manager repeat all admission checks. Never silently retarget a socket.
type watchedListener struct {
	net.Listener
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	err    error
	failed atomic.Bool
}

func watchListener(listener net.Listener, interval time.Duration, healthy func() bool) *watchedListener {
	watched := &watchedListener{Listener: listener, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(watched.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-watched.stop:
				return
			case <-ticker.C:
				if !healthy() {
					watched.failed.Store(true)
					watched.Close()
					return
				}
			}
		}
	}()
	return watched
}

func (listener *watchedListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if listener.failed.Load() {
		// A connection accepted concurrently with the failed observation must
		// not enter TLS/HTTP handling while the service is being retired.
		if connection != nil {
			connection.Close()
		}
		return nil, fmt.Errorf("private ingress interface changed; listener admission must restart: %w", errors.Join(net.ErrClosed, ErrIdentity, err))
	}
	return connection, err
}

func (listener *watchedListener) Close() error {
	listener.once.Do(func() {
		close(listener.stop)
		listener.err = listener.Listener.Close()
	})
	return listener.err
}
