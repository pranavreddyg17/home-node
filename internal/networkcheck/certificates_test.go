package networkcheck

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestCertificateReplacementAndExpiry(t *testing.T) {
	now := time.Now()
	current := tls.Certificate{Leaf: &x509.Certificate{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}}
	loads := 0
	var failure error
	source := &CertificateSource{now: func() time.Time { return now }, load: func() (tls.Certificate, error) { loads++; return current, failure }}
	first, err := source.GetCertificate(nil)
	if err != nil || loads != 1 {
		t.Fatal(err, loads)
	}
	current.Leaf = &x509.Certificate{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(2 * time.Hour)}
	now = now.Add(time.Minute)
	next, err := source.GetCertificate(nil)
	if err != nil || next.Leaf == first.Leaf || loads != 2 {
		t.Fatal("renewal not observed", err, loads)
	}
	failure = errors.New("invalid replacement")
	now = now.Add(time.Minute)
	if _, err = source.GetCertificate(nil); err == nil {
		t.Fatal("bad replacement used cached identity")
	}
	if _, err = source.GetCertificate(nil); err == nil || loads != 3 {
		t.Fatal("failed reload was not bounded", err, loads)
	}
	failure = nil
	now = now.Add(time.Minute)
	if _, err = source.GetCertificate(nil); err != nil {
		t.Fatal("repaired identity not loaded", err)
	}
	now = current.Leaf.NotAfter
	if _, err = source.GetCertificate(nil); err == nil {
		t.Fatal("expired identity accepted")
	}
}

func TestTLSFilesRejectLinksAndSpecialFiles(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "regular")
	if err := os.WriteFile(file, []byte("not a PEM"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(directory, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{directory, link, fifo} {
		if _, err := protectedPEM(name, true); err == nil {
			t.Fatal("unsafe TLS path accepted", name)
		}
	}
	if err := os.Chmod(file, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedPEM(file, true); err == nil {
		t.Fatal("writable identity accepted")
	}
}

func TestHTTPSHandshakeUsesValidatedIdentityAndFailsClosed(t *testing.T) {
	now := time.Now()
	certPEM, keyPEM, roots := testIdentity(t, now)
	certificate, err := Validate(Config{Bind: "100.100.1.2", Port: 8787, Origin: "https://home.example.ts.net:8787"}, []netip.Addr{netip.MustParseAddr("100.100.1.2")}, certPEM, keyPEM, roots, now)
	if err != nil {
		t.Fatal(err)
	}
	var clock atomic.Int64
	clock.Store(now.Unix())
	var invalid atomic.Bool
	source := &CertificateSource{now: func() time.Time { return time.Unix(clock.Load(), 0) }, load: func() (tls.Certificate, error) {
		if invalid.Load() {
			return tls.Certificate{}, ErrIdentity
		}
		return certificate, nil
	}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(204) }), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true, GetCertificate: source.GetCertificate}, ErrorLog: log.New(io.Discard, "", 0)}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "home.example.ts.net", MinVersion: tls.VersionTLS13, ClientSessionCache: tls.NewLRUClientSessionCache(4)}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	address := "https://" + listener.Addr().String()
	response, err := client.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 204 || requests.Load() != 1 {
		t.Fatal("validated handshake failed", response.StatusCode)
	}
	invalid.Store(true)
	clock.Store(now.Add(time.Minute).Unix())
	if response, err = client.Get(address); err == nil {
		response.Body.Close()
		t.Fatal("bad replacement reached handler")
	}
	if requests.Load() != 1 {
		t.Fatal("invalid identity allowed HTTP request")
	}
}

// Root ownership is part of production admission. Run this on a disposable
// Linux runner; ordinary developer runs must not change file ownership.
func TestRootTLSFileAdmission(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root-owned temporary identity files")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	payload := []byte("fixture identity bytes")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0600, 0640} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		got, err := protectedPEM(path, true)
		if err != nil || string(got) != string(payload) {
			t.Fatal("protected key refused", mode, err)
		}
	}

	if err := os.Chmod(dir, 0750); err != nil {
		t.Fatal(err)
	}
	group := uint32(os.Getegid())
	wrongGroup := group ^ 1
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedPEMWithGroup(path, true, &group); err == nil {
		t.Fatal("public TLS directory admitted")
	}
	if err := os.Chmod(dir, 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedPEMWithGroup(path, true, &group); err != nil {
		t.Fatal("gateway group refused", err)
	}
	if _, err := protectedPEMWithGroup(path, true, &wrongGroup); err == nil {
		t.Fatal("foreign key group admitted")
	}
	for _, mode := range []os.FileMode{0644, 0400, 0610, 0660, 0600 | os.ModeSetuid, 0600 | os.ModeSetgid, 0600 | os.ModeSticky} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := protectedPEM(path, true); err == nil {
			t.Fatal("unsafe key admitted", mode)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedPEM(path, true); err == nil {
		t.Fatal("hard-linked key admitted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedPEM(path, true); err == nil {
		t.Fatal("empty key admitted")
	}
}
