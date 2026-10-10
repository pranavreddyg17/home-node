// Package connectionbudget bounds accepted TCP connections before TLS parsing.
package connectionbudget

import (
	"errors"
	"net"
	"net/netip"
	"sync"
)

const DefaultTotal = 64
const DefaultPerAddress = 8

// Listener admits a bounded number of connections per source address and in
// total. Rejected connections are closed before entering the HTTP/TLS server.
// Request/stream authorization and deadlines remain independent requirements.
type Listener struct {
	net.Listener
	mu                 sync.Mutex
	closed             bool
	total              int
	perAddress         map[netip.Addr]int
	maximum, perSource int
}

func New(listener net.Listener, maximum, perSource int) (*Listener, error) {
	if listener == nil || maximum < 1 || maximum > 4096 || perSource < 1 || perSource > maximum {
		return nil, errors.New("invalid TCP connection budget")
	}
	return &Listener{Listener: listener, maximum: maximum, perSource: perSource, perAddress: make(map[netip.Addr]int)}, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		endpoint, err := netip.ParseAddrPort(conn.RemoteAddr().String())
		if err != nil {
			_ = conn.Close()
			continue
		}
		address := endpoint.Addr().Unmap()
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			_ = conn.Close()
			return nil, net.ErrClosed
		}
		if l.total >= l.maximum || l.perAddress[address] >= l.perSource {
			l.mu.Unlock()
			_ = conn.Close()
			continue
		}
		l.total++
		l.perAddress[address]++
		l.mu.Unlock()
		return &connection{Conn: conn, release: func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			l.perAddress[address]--
			if l.perAddress[address] == 0 {
				delete(l.perAddress, address)
			}
		}}, nil
	}
}

func (l *Listener) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return l.Listener.Close()
}

type connection struct {
	net.Conn
	once    sync.Once
	release func()
	err     error
}

func (c *connection) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.release() })
	return c.err
}
