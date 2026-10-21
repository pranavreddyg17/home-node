//go:build linux

package main

import (
	"bufio"
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
	"strings"
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
	var idempotencyKey string
	var traceNext, reused bool
	var authWindow time.Time
	var authRequests int
	request := func(method, path string, payload any, status int) []byte {
		t.Helper()
		// The joined workflow adds app approval ceremonies to the identity
		// sequence. Pace this disposable client within the public 30/minute
		// authentication budget; never disable the production limiter or
		// silently retry a signed mutation after a refusal.
		if os.Getenv("HOMENODE_NATIVE_HTTPS_FILES") == "1" && (strings.HasPrefix(path, "/api/v1/auth/") || strings.HasSuffix(path, "/approval")) {
			if authWindow.IsZero() || time.Since(authWindow) >= time.Minute+time.Second {
				authWindow, authRequests = time.Now(), 0
			}
			ceremonyStart := strings.HasSuffix(path, "/begin") || strings.HasSuffix(path, "/approval") || path == "/api/v1/auth/recover"
			if authRequests >= 28 || authRequests >= 26 && ceremonyStart {
				if delay := time.Until(authWindow.Add(time.Minute + time.Second)); delay > 0 {
					time.Sleep(delay)
				}
				authWindow, authRequests = time.Now(), 0
			}
			authRequests++
		}
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
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
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
		started := time.Now()
		response, err := client.Do(req)
		if err != nil {
			t.Fatalf("HTTPS request failed for %s: %v", path, err)
		}
		defer response.Body.Close()
		if response.StatusCode != status || response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || len(response.TLS.VerifiedChains) == 0 {
			t.Fatalf("unexpected verified HTTPS result for %s: status %d, expected %d, elapsed %s", path, response.StatusCode, status, time.Since(started).Round(time.Millisecond))
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
	var recoveryCodes []string
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
		if recoveryCount > 0 {
			recoveryCodes = append([]string{}, result.RecoveryCodes...)
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
	var activeSessions []struct {
		DeviceID  string `json:"deviceId"`
		Current   bool   `json:"current"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	inventory := request(http.MethodGet, "/api/v1/sessions", nil, 200)
	if json.Unmarshal(inventory, &activeSessions) != nil || len(activeSessions) != 1 || !activeSessions[0].Current || activeSessions[0].DeviceID == "" || activeSessions[0].ExpiresAt <= time.Now().Unix() || bytes.Contains(bytes.ToLower(inventory), []byte("token")) {
		t.Fatal("native administrator session inventory invalid or exposed credentials")
	}
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
	var files *nativeHTTPSFilesResult
	if os.Getenv("HOMENODE_NATIVE_HTTPS_FILES") == "1" {
		files = nativeHTTPSFilesRoundTrip(t, client, origin, request, approve, &actionApproval, &idempotencyKey)
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
	request(http.MethodGet, "/api/v1/sessions", nil, 403)
	sessionData := request(http.MethodGet, "/api/v1/session", nil, 200)
	var pairedSession struct{ Device struct{ ID string } }
	if json.Unmarshal(sessionData, &pairedSession) != nil || pairedSession.Device.ID == "" {
		t.Fatal("paired device session missing")
	}
	finalizationID := ""
	if files != nil && files.Size == 1<<30 {
		finalizationID = prepareNativeRevocableFinalization(t, request)
		client = owner
		files.RestartGuest()
		client = paired
	}
	streamContext, cancelStream := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelStream()
	streamRequest, err := http.NewRequestWithContext(streamContext, http.MethodGet, origin+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamRequest.Header.Set("Origin", origin)
	streamClient := &http.Client{Transport: transport, Jar: pairedJar}
	stream, err := streamClient.Do(streamRequest)
	if err != nil {
		t.Fatal("paired HTTPS event stream failed to open")
	}
	defer stream.Body.Close()
	if stream.StatusCode != 200 || stream.TLS == nil || len(stream.TLS.VerifiedChains) == 0 || stream.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("paired event stream did not establish verified HTTPS")
	}
	reader := bufio.NewReader(stream.Body)
	readHeartbeat := func() {
		t.Helper()
		for line := 0; line < 200; line++ {
			data, err := reader.ReadSlice('\n')
			if err != nil {
				t.Fatal("paired event stream ended before heartbeat")
			}
			if bytes.Equal(bytes.TrimSpace(data), []byte(": heartbeat")) {
				return
			}
		}
		t.Fatal("paired event stream did not publish initial heartbeat")
	}
	readHeartbeat()
	client = owner
	revokePath := "/api/v1/devices/" + pairedSession.Device.ID + "/revoke"
	empty := map[string]any{}
	request(http.MethodPost, revokePath, empty, 403)
	actionApproval = approve(revokePath, empty)
	readHeartbeat() // still active after approval, immediately before revocation
	var dataStream *nativeRevocableDownload
	var uploadStream *nativeRevocableUpload
	if files != nil {
		uploadStream = startNativeRevocableUpload(t, paired, origin)
		defer uploadStream.Close()
	}
	if files != nil && files.Size == 1<<30 {
		dataStream = startNativeRevocableDownload(t, paired, origin, files)
		defer dataStream.Close()
	}
	var finalization *nativeRevocableFinalization
	if finalizationID != "" {
		finalization = startNativeRevocableFinalization(t, paired, origin, finalizationID)
		defer finalization.cancel()
	}
	request(http.MethodPost, revokePath, empty, 200)
	actionApproval = ""
	remaining, err := io.ReadAll(io.LimitReader(reader, (128<<10)+1))
	if err != nil || len(remaining) > 128<<10 || streamContext.Err() != nil {
		t.Fatal("revoked device event stream did not end before its normal lifetime")
	}
	stream.Body.Close()
	client = paired
	traceNext = true
	request(http.MethodGet, "/api/v1/files", nil, 401)
	if !reused {
		t.Fatal("revocation did not exercise a reused verified TLS connection")
	}
	traceNext = false
	client = owner
	if files != nil {
		uploadStream.ExpectRevoked(t)
		if finalization != nil {
			finalization.ExpectRevoked(t)
		}
		if dataStream != nil {
			dataStream.ExpectRevoked(t)
			files.VerifyOwner()
			t.Log("native HTTPS retained owner file integrity after paired revocation passed")
		}
		if files.Size == 1<<30 {
			nativeHTTPSFilesCapacityRecovery(t, owner, origin, request, files)
		}
		files.Stop()
	}
	oldRecovery, unusedOldRecovery := recoveryCodes[0], recoveryCodes[1]
	recoveryData := request(http.MethodPost, "/api/v1/auth/recover", map[string]string{"code": oldRecovery}, 200)
	request(http.MethodPost, "/api/v1/auth/recover", map[string]string{"code": oldRecovery}, 403)
	request(http.MethodGet, "/api/v1/devices", nil, 200) // replacement has not committed
	var replacementInvitation struct{ Code string }
	if json.Unmarshal(recoveryData, &replacementInvitation) != nil || replacementInvitation.Code == "" {
		t.Fatal("replacement enrollment invitation missing")
	}
	replacementJar, _ := cookiejar.New(nil)
	replacement := &http.Client{Transport: transport, Jar: replacementJar, Timeout: 5 * time.Second}
	client = replacement
	newKey, newCredential, newHandle := enroll(replacementInvitation.Code, "Recovered HTTPS owner", 8)
	if recoveryCodes[0] == oldRecovery || recoveryCodes[0] == unusedOldRecovery {
		t.Fatal("replacement identity reused old recovery material")
	}
	request(http.MethodGet, "/api/v1/devices", nil, 200)
	request(http.MethodPost, "/api/v1/auth/recover", map[string]string{"code": unusedOldRecovery}, 403)
	client = owner
	request(http.MethodGet, "/api/v1/devices", nil, 401)
	begin := request(http.MethodPost, "/api/v1/auth/login/begin", map[string]any{}, 200)
	var retiredOptions struct{ PublicKey struct{ Challenge string } }
	if json.Unmarshal(begin, &retiredOptions) != nil || retiredOptions.PublicKey.Challenge == "" {
		t.Fatal("retired credential test challenge missing")
	}
	request(http.MethodPost, "/api/v1/auth/login/finish", sign(retiredOptions.PublicKey.Challenge, origin, 5, counter+1), 403)
	key, credential, userHandle = newKey, newCredential, newHandle
	client = replacement
	request(http.MethodPost, "/api/v1/logout", map[string]any{}, 200)
	begin = request(http.MethodPost, "/api/v1/auth/login/begin", map[string]any{}, 200)
	var replacementOptions struct{ PublicKey struct{ Challenge string } }
	if json.Unmarshal(begin, &replacementOptions) != nil || replacementOptions.PublicKey.Challenge == "" {
		t.Fatal("replacement credential login challenge missing")
	}
	request(http.MethodPost, "/api/v1/auth/login/finish", sign(replacementOptions.PublicKey.Challenge, origin, 5, 1), 200)
	request(http.MethodGet, "/api/v1/devices", nil, 200)
	if descriptor := os.Getenv("HOMENODE_NATIVE_HTTPS_BROWSER_FD"); descriptor != "" {
		if files == nil {
			t.Fatal("native browser handoff requires the actual Files backend")
		}
		body := map[string]any{"name": "Native browser device", "capabilities": []string{"files", "admin"}}
		actionApproval = approve("/api/v1/devices/pair", body)
		encoded := request(http.MethodPost, "/api/v1/devices/pair", body, 201)
		actionApproval = ""
		var browserInvitation struct{ Code string }
		if json.Unmarshal(encoded, &browserInvitation) != nil || browserInvitation.Code == "" {
			t.Fatal("browser pairing did not return an invitation")
		}
		if err := writeNativeBrowserInvitation(descriptor, browserInvitation.Code); err != nil {
			t.Fatal("private browser invitation handoff failed", err)
		}
		t.Log("native HTTPS browser invitation handed to private pipe")
	}
}
