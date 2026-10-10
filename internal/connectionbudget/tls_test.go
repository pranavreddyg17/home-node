package connectionbudget

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestBudgetBeforeTLSAndHTTPSRecovery(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Error("request was not TLS 1.3")
		}
		_, _ = w.Write([]byte("healthy"))
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	listener, err := New(server.Listener, 1, 1)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Listener = listener
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	first, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	waitCount := func(want int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			listener.mu.Lock()
			count := listener.total
			listener.mu.Unlock()
			if count == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("connection count did not settle", want)
	}
	waitCount(1)
	denied, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", listener.Addr().String(), tlsConfig)
	if err == nil {
		denied.Close()
		t.Fatal("excess connection completed TLS handshake")
	}
	if requests.Load() != 0 {
		t.Fatal("idle or excess connection reached HTTP handler")
	}
	first.Close()
	waitCount(0)
	transport := &http.Transport{TLSClientConfig: tlsConfig, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal("HTTPS did not recover after slot release", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64))
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "healthy" || requests.Load() != 1 {
		t.Fatal("unexpected HTTPS response", response.StatusCode, string(body), err)
	}
}
