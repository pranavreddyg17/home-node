package control

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s, err := New(store, Config{Origin: "http://localhost:8787", Development: true, Report: func() hostcheck.Report { return hostcheck.Report{} }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func request(s *Server, method, target, body, token, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.URL.Scheme = ""
	r.URL.Host = ""
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func seedSession(t *testing.T, s *Server, caps string) string {
	t.Helper()
	token := state.Random()
	now := time.Now().Unix()
	err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES('device','Laptop',?,?)", caps, now); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO sessions VALUES(?,'device',1,?,?,?,?)", state.Hash(token), now, now, now, now+3600)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func TestHTTPBoundaries(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct {
		name, method, url, origin string
		want                      int
	}{
		{"private report", "GET", "http://localhost:8787/api/v1/host/report", "", 401},
		{"wrong host", "GET", "http://evil.example/api/v1/setup", "", 403},
		{"wrong port", "GET", "http://localhost:9000/api/v1/setup", "", 403},
		{"missing origin", "POST", "http://localhost:8787/api/v1/auth/login/begin", "", 403},
		{"cross origin", "POST", "http://localhost:8787/api/v1/auth/login/begin", "http://evil.example", 403},
		{"status", "GET", "http://localhost:8787/api/v1/setup", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(s, tc.method, tc.url, "{}", "", tc.origin)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		})
	}
}
func TestCapabilitiesAndLogout(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	if w := request(s, "GET", "http://localhost:8787/api/v1/devices", "", token, ""); w.Code != 403 {
		t.Fatalf("limited session accessed devices: %d", w.Code)
	}
	if w := request(s, "GET", "http://localhost:8787/api/v1/host/report", "", token, ""); w.Code != 200 {
		t.Fatalf("report: %d", w.Code)
	}
	if w := request(s, "POST", "http://localhost:8787/api/v1/logout", "{}", token, s.config.Origin); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := request(s, "GET", "http://localhost:8787/api/v1/session", "", token, ""); w.Code != 401 {
		t.Fatalf("logged out session accepted: %d", w.Code)
	}
}
func TestStrictJSON(t *testing.T) {
	s := testServer(t)
	for _, body := range []string{`{"code":"x","name":"test","admin":true}`, `{} {}`, strings.Repeat("x", 65537)} {
		w := request(s, "POST", "http://localhost:8787/api/v1/auth/register/begin", body, "", s.config.Origin)
		if w.Code != 400 {
			t.Fatalf("malformed body got %d", w.Code)
		}
	}
}
func TestEnrollmentChallengeCookie(t *testing.T) {
	s := testServer(t)
	code, err := s.Identity.CreateSetupCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"code": code, "name": "Laptop"})
	w := request(s, "POST", "http://localhost:8787/api/v1/auth/register/begin", string(data), "", s.config.Origin)
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge != 300 {
		t.Fatal("incorrect challenge cookie")
	}
}
func TestProductionTLSAndCookie(t *testing.T) {
	s := testServer(t)
	s.config.Development = false
	s.config.Origin = "https://node.example"
	s.host = "node.example"
	s.cookie = "__Host-homenode"
	r := httptest.NewRequest("GET", "https://node.example/api/v1/setup", nil)
	r.URL.Scheme = ""
	r.URL.Host = ""
	r.TLS = nil
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("plaintext production accepted")
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-Proto", "X-Homenode-Client-TLS"} {
		r.Header.Set(header, "1.3")
	}
	r.Header.Set("X-Homenode-Client-Address", "100.64.0.2")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("public headers bypassed production HTTPS guard")
	}
	r.TLS = &tls.ConnectionState{}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("TLS path failed")
	}
	w = httptest.NewRecorder()
	s.setCookie(w, "", "secret", 300)
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || cookie.Domain != "" || cookie.Path != "/" {
		t.Fatal("cookie boundary invalid")
	}
}
func TestOriginCannotChangeSilently(t *testing.T) {
	s := testServer(t)
	_, err := New(s.Store, Config{Origin: "http://localhost:9000", Development: true})
	if err == nil {
		t.Fatal("RP origin changed without recovery")
	}
}
