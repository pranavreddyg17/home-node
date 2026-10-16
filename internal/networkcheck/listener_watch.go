package networkcheck

import (
	"net"
	"sync"
	"time"
)

// A changed interface must cause the installed gateway to exit and let its
// service manager repeat all admission checks. Never silently retarget a socket.
type watchedListener struct {
	net.Listener
	stop chan struct{}
	done chan struct{}
	once sync.Once
	err  error
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
					watched.Close()
					return
				}
			}
		}
	}()
	return watched
}

func (listener *watchedListener) Close() error {
	listener.once.Do(func() {
		close(listener.stop)
		listener.err = listener.Listener.Close()
	})
	return listener.err
}
