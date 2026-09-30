package control

import (
	"context"
	"encoding/json"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestApprovalHTTPAuthenticationAndBoundedBegin(t *testing.T) {
	s := testServer(t)
	s.config.PolicyGeneration = 1
	origin := "http://localhost:8787"
	path := origin + "/api/v1/devices/pair/approval"
	if w := request(s, "POST", path, `{}`, "", origin); w.Code != 401 {
		t.Fatal("unauthenticated approval admitted", w.Code)
	}
	token := seedSession(t, s, `["admin"]`)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	credential := webauthn.Credential{ID: []byte("credential"), PublicKey: []byte("synthetic-begin-only")}
	data, _ := json.Marshal(credential)
	if _, err := s.Store.DB.Exec("INSERT INTO credentials(id,device_id,data) VALUES(?,'device',?)", credential.ID, string(data)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"Name":"phone","capabilities":["files"]}`, `{"name":"a","name":"b","capabilities":["files"]}`, strings.Repeat("x", 4097)} {
		if w := request(s, "POST", path, body, token, origin); w.Code == 200 {
			t.Fatal("invalid body admitted", body)
		}
	}
	w := request(s, "POST", path, `{"name":"phone","capabilities":["files"]}`, token, origin)
	if w.Code != 200 {
		t.Fatal("valid begin refused", w.Code, w.Body.String())
	}
	var result struct {
		Options        protocol.CredentialAssertion `json:"options"`
		ChallengeToken string                       `json:"challengeToken"`
		ExpiresAt      int64                        `json:"expiresAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.ChallengeToken == "" || result.ExpiresAt == 0 || result.Options.Response.UserVerification != protocol.VerificationRequired {
		t.Fatal("invalid begin contract", err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("approval token cacheable")
	}
	if w := request(s, "POST", origin+"/api/v1/auth/approval/finish", `{}`, token, origin); w.Code != 403 {
		t.Fatal("missing challenge issued grant", w.Code)
	}
}

func TestDeviceMutationsRequireActionGrantEvenWithFreshSession(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["admin"]`)
	origin := "http://localhost:8787"
	for _, path := range []string{"/api/v1/devices/pair", "/api/v1/devices/device/revoke"} {
		w := request(s, "POST", origin+path, `{}`, token, origin)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "APPROVAL_REQUIRED") {
			t.Fatal("fresh session bypassed approval", path, w.Code, w.Body.String())
		}
	}
}

func TestAppActionConsumesExactGrantWithDurableIntent(t *testing.T) {
	s := testServer(t)
	s.config.PolicyGeneration = 1
	backend := fileBackend{}
	s.config.Runtime = backend
	s.Workloads = workload.New(s.Store, backend, 1)
	session := seedSession(t, s, `["admin"]`)
	actor, err := s.Identity.Authenticate(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	key := state.Random()
	_, resources, err := s.appApprovalResources(context.Background(), "files", key)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"action":"start"}`
	grant := state.Random()
	binding := identity.ApprovalBinding{Action: "app.start", Resources: resources, BodySHA256: state.Hash(body), PolicyGeneration: 1, Epoch: actor.Epoch, ExpiresAt: time.Now().Unix() + 120, DeviceID: actor.Device.ID, SessionHash: actor.TokenHash}
	// Production binding resources are canonicalized before persistence.
	slices.Sort(binding.Resources)
	payload, _ := json.Marshal(map[string]any{"binding": binding, "issuer": actor.Device.ID})
	if _, err := s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(grant), string(payload), actor.Epoch, binding.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	send := func(requestKey string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost:8787/api/v1/apps/files/actions", strings.NewReader(body))
		r.URL.Scheme = ""
		r.URL.Host = ""
		r.Header.Set("Origin", "http://localhost:8787")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", requestKey)
		r.Header.Set("X-Action-Approval", grant)
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: session})
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := send(state.Random()); w.Code != 403 {
		t.Fatal("changed request key authorized", w.Code, w.Body.String())
	}
	if w := send(key); w.Code != 202 {
		t.Fatal("approved intent failed", w.Code, w.Body.String())
	}
	if w := send(key); w.Code != 403 {
		t.Fatal("consumed approval replayed", w.Code, w.Body.String())
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM operations WHERE state='pending'").Scan(&count); err != nil || count != 1 {
		t.Fatal("wrong intent count", count, err)
	}
}
