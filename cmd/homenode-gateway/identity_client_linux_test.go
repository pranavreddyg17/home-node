//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// A generated protocol authenticator proves server verification across real
// HTTPS. It does not qualify physical user verification or browser passkey UX.
func TestNativeHTTPSIdentityClient(t *testing.T) {
	if os.Getenv("HOMENODE_NATIVE_HTTPS_IDENTITY") != "1" {
		t.Skip("disposable namespace executable fixture only")
	}
	var input struct{ RootPEM, SetupCode string }
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 32<<10)).Decode(&input); err != nil || input.SetupCode == "" {
		t.Fatal("missing bounded identity fixture input")
	}
	certificate, err := os.ReadFile(input.RootPEM)
	if err != nil {
		t.Fatal("fixture trust root unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("fixture trust root invalid")
	}
	const origin = "https://home.example.ts.net:8787"
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "home.example.ts.net:8787" {
			return nil, errors.New("unexpected fixture destination")
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", "100.100.1.2:8787")
	}}
	defer transport.CloseIdleConnections()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 5 * time.Second}
	request := func(method, path string, payload any, status int) []byte {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("fixture request encoding failed")
		}
		req, err := http.NewRequest(method, origin+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal("fixture request invalid")
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatalf("HTTPS request failed for %s: %v", path, err)
		}
		defer response.Body.Close()
		if response.StatusCode != status || response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || len(response.TLS.VerifiedChains) == 0 {
			t.Fatalf("unexpected verified HTTPS result for %s: status %d, expected %d", path, response.StatusCode, status)
		}
		if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Strict-Transport-Security") == "" {
			t.Fatalf("private response security headers missing for %s", path)
		}
		for _, cookie := range response.Cookies() {
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" || cookie.Path != "/" {
				t.Fatal("identity cookie protections missing")
			}
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
		if err != nil || len(data) > 64<<10 {
			t.Fatal("identity response exceeded fixture bound")
		}
		return data
	}
	request(http.MethodGet, "/api/v1/session", nil, 401)
	creation := request(http.MethodPost, "/api/v1/auth/register/begin", map[string]string{"code": input.SetupCode, "name": "Native HTTPS owner"}, 200)
	var options struct {
		PublicKey struct {
			Challenge string
			User      struct{ ID string }
		}
	}
	if json.Unmarshal(creation, &options) != nil || options.PublicKey.Challenge == "" || options.PublicKey.User.ID == "" {
		t.Fatal("registration options missing")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credential := make([]byte, 32)
	if _, err := rand.Read(credential); err != nil {
		t.Fatal(err)
	}
	public, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	rpHash := sha256.Sum256([]byte("home.example.ts.net"))
	authData := append(append([]byte{}, rpHash[:]...), 69, 0, 0, 0, 0) // UP, UV, AT
	authData = append(authData, make([]byte, 16)...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(credential)))
	authData = append(append(authData, credential...), public...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "authData": authData, "attStmt": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	clientData, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": options.PublicKey.Challenge, "origin": origin, "crossOrigin": false})
	registration := map[string]any{"id": enc(credential), "rawId": enc(credential), "type": "public-key", "response": map[string]any{"clientDataJSON": enc(clientData), "attestationObject": enc(attestation), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{}}
	finished := request(http.MethodPost, "/api/v1/auth/register/finish", registration, 201)
	var result struct{ RecoveryCodes []string }
	if json.Unmarshal(finished, &result) != nil || len(result.RecoveryCodes) != 8 {
		t.Fatal("owner enrollment did not return expected recovery material")
	}
	request(http.MethodGet, "/api/v1/devices", nil, 200)
	request(http.MethodPost, "/api/v1/logout", map[string]any{}, 200)
	request(http.MethodGet, "/api/v1/devices", nil, 401)
	for index, check := range []struct {
		origin string
		flags  byte
		status int
	}{{"https://foreign.example.ts.net:8787", 5, 403}, {origin, 1, 403}, {origin, 5, 200}} {
		begin := request(http.MethodPost, "/api/v1/auth/login/begin", map[string]any{}, 200)
		var assertionOptions struct{ PublicKey struct{ Challenge string } }
		if json.Unmarshal(begin, &assertionOptions) != nil || assertionOptions.PublicKey.Challenge == "" {
			t.Fatal("login options missing")
		}
		data, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": assertionOptions.PublicKey.Challenge, "origin": check.origin, "crossOrigin": false})
		auth := append(append([]byte{}, rpHash[:]...), check.flags)
		auth = binary.BigEndian.AppendUint32(auth, uint32(index+1))
		hash := sha256.Sum256(data)
		digest := sha256.Sum256(append(append([]byte{}, auth...), hash[:]...))
		signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		assertion := map[string]any{"id": enc(credential), "rawId": enc(credential), "type": "public-key", "response": map[string]string{"clientDataJSON": enc(data), "authenticatorData": enc(auth), "signature": enc(signature), "userHandle": options.PublicKey.User.ID}, "clientExtensionResults": map[string]any{}}
		request(http.MethodPost, "/api/v1/auth/login/finish", assertion, check.status)
	}
	request(http.MethodGet, "/api/v1/devices", nil, 200)
}
