package connectionbudget

import (
	"io"
	"net"
	"testing"
	"time"
)

// Actual TCP clients cover admission, rejection before application processing,
// and slot release. Source and total limits are exercised separately.
func TestTCPBudgetsAndRelease(t *testing.T) {
	for _, limits := range []struct {
		name          string
		total, source int
	}{{"global", 1, 1}, {"source", 2, 1}} {
		t.Run(limits.name, func(t *testing.T) {
			raw, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			listener, err := New(raw, limits.total, limits.source)
			if err != nil {
				raw.Close()
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 4)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					accepted <- conn
				}
			}()
			defer func() {
				listener.Close()
				<-done
				close(accepted)
				for conn := range accepted {
					conn.Close()
				}
			}()
			dial := func() net.Conn {
				t.Helper()
				conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { conn.Close() })
				return conn
			}
			receive := func() net.Conn {
				t.Helper()
				select {
				case conn := <-accepted:
					return conn
				case <-time.After(2 * time.Second):
					t.Fatal("connection not admitted")
					return nil
				}
			}
			first := dial()
			server := receive()
			defer server.Close()
			rejected := dial()
			rejected.SetReadDeadline(time.Now().Add(2 * time.Second))
			buffer := make([]byte, 1)
			if _, err := rejected.Read(buffer); err != io.EOF {
				t.Fatal("excess connection was not closed before application work", err)
			}
			select {
			case conn := <-accepted:
				conn.Close()
				t.Fatal("excess connection reached application")
			default:
			}
			server.Close()
			server.Close() // Duplicate closes must not return an extra budget slot.
			first.Close()
			next := dial()
			replacement := receive()
			defer replacement.Close()
			replacement.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := replacement.Write([]byte("ok")); err != nil {
				t.Fatal(err)
			}
			next.SetReadDeadline(time.Now().Add(time.Second))
			result := make([]byte, 2)
			if _, err := io.ReadFull(next, result); err != nil || string(result) != "ok" {
				t.Fatal("admitted connection was not usable", err)
			}
			stillRejected := dial()
			stillRejected.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := stillRejected.Read(buffer); err != io.EOF {
				t.Fatal("duplicate close enlarged budget", err)
			}
		})
	}
}

func TestInvalidBudgets(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, limits := range [][2]int{{0, 1}, {1, 0}, {1, 2}, {4097, 1}} {
		if _, err := New(raw, limits[0], limits[1]); err == nil {
			t.Fatal("invalid limits admitted", limits)
		}
	}
	if _, err := New(nil, 1, 1); err == nil {
		t.Fatal("nil listener admitted")
	}
}

// Distinct source identities isolate the global limit from the per-address
// limit. Pipe transport avoids changing local network interface configuration.
type addressedConnection struct {
	net.Conn
	address net.Addr
}

func (c addressedConnection) RemoteAddr() net.Addr { return c.address }

type queuedListener struct {
	connections chan net.Conn
	done        chan struct{}
}

func (l *queuedListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connections:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *queuedListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}
func (l *queuedListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10000}
}

func TestGlobalBudgetAcrossDistinctSources(t *testing.T) {
	raw := &queuedListener{connections: make(chan net.Conn, 3), done: make(chan struct{})}
	listener, err := New(raw, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	makeConnection := func(ip string) net.Conn {
		server, client := net.Pipe()
		t.Cleanup(func() { server.Close(); client.Close() })
		raw.connections <- addressedConnection{server, &net.TCPAddr{IP: net.ParseIP(ip), Port: 12345}}
		return client
	}
	first := makeConnection("100.64.0.1")
	admitted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer admitted.Close()
	rejected := makeConnection("100.64.0.2")
	result := make(chan net.Conn, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			result <- conn
		}
	}()
	defer func() {
		listener.Close()
		<-done
		select {
		case conn := <-result:
			conn.Close()
		default:
		}
	}()
	rejected.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := rejected.Read(make([]byte, 1)); err != io.EOF {
		t.Fatal("global limit admitted another source", err)
	}
	admitted.Close()
	first.Close()
	next := makeConnection("100.64.0.3")
	next.SetReadDeadline(time.Now().Add(time.Second))
	select {
	case conn := <-result:
		conn.Close()
	case <-time.After(time.Second):
		t.Fatal("global slot was not released")
	}
}
