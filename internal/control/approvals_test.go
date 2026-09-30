package control

import (
	"encoding/json"
	"strings"
	"testing"

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
