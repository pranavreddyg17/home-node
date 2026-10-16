package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListenerFailureClosesActiveTLSAfterGrace(t *testing.T) {
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := fixture.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	fixture.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entered, cancelled := make(chan struct{}), make(chan struct{})
	server := &http.Server{TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13}, Handler: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
		close(cancelled)
	})}
	defer server.Close()
	result := make(chan error, 1)
	go func() { result <- servePrivateHTTPS(context.Background(), server, listener, 20*time.Millisecond) }()
	client, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp4", listener.Addr().String(), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(client, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("TLS request did not enter handler")
	}
	listener.Close()
	select {
	case err := <-result:
		if !errors.Is(err, net.ErrClosed) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("listener failure must remain an unsuccessful exit with exhausted drain: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active TLS request prevented bounded gateway exit")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("forced server close did not cancel the active request")
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("active TLS connection remained readable after forced close")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("active TLS connection remained open after gateway exit")
	}
}
