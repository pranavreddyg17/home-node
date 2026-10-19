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
	"net/http/httptrace"
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
	var approvalChallenge, actionApproval string
	var traceNext, reused bool
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
		if approvalChallenge != "" {
			req.Header.Set("X-Approval-Challenge", approvalChallenge)
		}
		if actionApproval != "" {
			req.Header.Set("X-Action-Approval", actionApproval)
		}
		if traceNext {
			reused = false
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}))
		}
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
	enc := base64.RawURLEncoding.EncodeToString
	rpHash := sha256.Sum256([]byte("home.example.ts.net"))
	enroll := func(code, deviceName string, recoveryCount int) (*ecdsa.PrivateKey, []byte, string) {
		t.Helper()
		creation := request(http.MethodPost, "/api/v1/auth/register/begin", map[string]string{"code": code, "name": deviceName}, 200)
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
		if json.Unmarshal(finished, &result) != nil || len(result.RecoveryCodes) != recoveryCount {
			t.Fatal("owner enrollment did not return expected recovery material")
		}
		return key, credential, options.PublicKey.User.ID
	}
	key, credential, userHandle := enroll(input.SetupCode, "Native HTTPS owner", 8)
	request(http.MethodGet, "/api/v1/devices", nil, 200)
	request(http.MethodPost, "/api/v1/logout", map[string]any{}, 200)
	request(http.MethodGet, "/api/v1/devices", nil, 401)
	sign := func(challenge, clientOrigin string, flags byte, counter uint32) map[string]any {
		t.Helper()
		data, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": challenge, "origin": clientOrigin, "crossOrigin": false})
		auth := append(append([]byte{}, rpHash[:]...), flags)
		auth = binary.BigEndian.AppendUint32(auth, counter)
		hash := sha256.Sum256(data)
		digest := sha256.Sum256(append(append([]byte{}, auth...), hash[:]...))
		signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{"id": enc(credential), "rawId": enc(credential), "type": "public-key", "response": map[string]string{"clientDataJSON": enc(data), "authenticatorData": enc(auth), "signature": enc(signature), "userHandle": userHandle}, "clientExtensionResults": map[string]any{}}
	}
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
		assertion := sign(assertionOptions.PublicKey.Challenge, check.origin, check.flags, uint32(index+1))
		request(http.MethodPost, "/api/v1/auth/login/finish", assertion, check.status)
	}
	request(http.MethodGet, "/api/v1/devices", nil, 200)
	owner := client
	counter := uint32(3)
	approve := func(path string, payload any) string {
		t.Helper()
		data := request(http.MethodPost, path+"/approval", payload, 200)
		var ceremony struct {
			Options        struct{ PublicKey struct{ Challenge string } }
			ChallengeToken string
		}
		if json.Unmarshal(data, &ceremony) != nil || ceremony.Options.PublicKey.Challenge == "" || ceremony.ChallengeToken == "" {
			t.Fatal("sensitive approval options missing")
		}
		counter++
		approvalChallenge = ceremony.ChallengeToken
		finished := request(http.MethodPost, "/api/v1/auth/approval/finish", sign(ceremony.Options.PublicKey.Challenge, origin, 5, counter), 200)
		approvalChallenge = ""
		var grant struct{ ApprovalToken string }
		if json.Unmarshal(finished, &grant) != nil || grant.ApprovalToken == "" {
			t.Fatal("signed sensitive approval did not return grant")
		}
		return grant.ApprovalToken
	}
	pairBody := map[string]any{"name": "Native Files device", "capabilities": []string{"files"}}
	request(http.MethodPost, "/api/v1/devices/pair", pairBody, 403)
	actionApproval = approve("/api/v1/devices/pair", pairBody)
	request(http.MethodPost, "/api/v1/devices/pair", map[string]any{"name": "Changed invitation", "capabilities": []string{"files"}}, 403)
	invitationData := request(http.MethodPost, "/api/v1/devices/pair", pairBody, 201)
	request(http.MethodPost, "/api/v1/devices/pair", pairBody, 403) // consumed grant
	actionApproval = ""
	var invitation struct{ Code string }
	if json.Unmarshal(invitationData, &invitation) != nil || invitation.Code == "" {
		t.Fatal("approved device invitation missing")
	}
	pairedJar, _ := cookiejar.New(nil)
	paired := &http.Client{Transport: transport, Jar: pairedJar, Timeout: 5 * time.Second}
	client = paired
	enroll(invitation.Code, "Native Files device", 0)
	request(http.MethodGet, "/api/v1/files", nil, 200)
	request(http.MethodGet, "/api/v1/devices", nil, 403)
	sessionData := request(http.MethodGet, "/api/v1/session", nil, 200)
	var pairedSession struct{ Device struct{ ID string } }
	if json.Unmarshal(sessionData, &pairedSession) != nil || pairedSession.Device.ID == "" {
		t.Fatal("paired device session missing")
	}
	client = owner
	revokePath := "/api/v1/devices/" + pairedSession.Device.ID + "/revoke"
	empty := map[string]any{}
	request(http.MethodPost, revokePath, empty, 403)
	actionApproval = approve(revokePath, empty)
	request(http.MethodPost, revokePath, empty, 200)
	actionApproval = ""
	client = paired
	traceNext = true
	request(http.MethodGet, "/api/v1/files", nil, 401)
	if !reused {
		t.Fatal("revocation did not exercise a reused verified TLS connection")
	}
}
