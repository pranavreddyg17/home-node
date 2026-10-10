package gatewaytransport

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicAssertionsCannotCreateTrustedTransport(t *testing.T) {
	called := false
	handler, err := ControllerHandler(1001, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(clientAddressHeader, "100.64.0.2")
	request.Header.Set(clientTLSHeader, "1.3")
	request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 403 || called || TrustedHTTPS(request) {
		t.Fatal("headers bypassed kernel peer authentication")
	}
}

func TestProxyRefusesUnsafeFrontendRequests(t *testing.T) {
	handler, close, err := NewProxy(Config{Origin: "https://home.example.test:8787", Socket: "/tmp/absent-controller.sock", ControllerUID: 1001, AccessGID: 1001})
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	for _, change := range []struct {
		name  string
		apply func(*http.Request)
	}{
		{"plaintext", func(r *http.Request) { r.TLS = nil; r.Header.Set(clientTLSHeader, "1.3") }},
		{"old TLS", func(r *http.Request) { r.TLS.Version = tls.VersionTLS12 }},
		{"foreign host", func(r *http.Request) { r.Host = "other.example.test:8787" }},
		{"public source", func(r *http.Request) { r.RemoteAddr = "8.8.8.8:12345" }},
		{"absolute target", func(r *http.Request) { r.URL.Scheme = "https"; r.URL.Host = "other.example.test" }},
		{"tunnel", func(r *http.Request) { r.Method = "CONNECT" }},
		{"upgrade", func(r *http.Request) { r.Header.Set("Upgrade", "websocket") }},
	} {
		t.Run(change.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/v1/healthz", nil)
			request.Host = "home.example.test:8787"
			request.RemoteAddr = "100.64.0.2:12345"
			request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
			change.apply(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 403 {
				t.Fatal("unsafe frontend accepted", response.Code)
			}
		})
	}
}
