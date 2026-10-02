package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestBackupApprovalRequiresTrustedTargetAndAdministrator(t *testing.T) {
	s := testServer(t)
	s.config.PolicyGeneration = 1
	origin := "http://localhost:8787"
	path := origin + "/api/v1/backups/approval"
	if got := request(s, "POST", path, `{}`, "", origin); got.Code != 401 {
		t.Fatal(got.Code)
	}
	limitedServer := testServer(t)
	limited := seedSession(t, limitedServer, `["files"]`)
	if got := request(limitedServer, "POST", path, `{}`, limited, origin); got.Code != 403 {
		t.Fatal(got.Code)
	}
	token := seedSession(t, s, `["admin"]`)
	if got := request(s, "POST", path, `{}`, token, origin); got.Code != 503 {
		t.Fatal(got.Code)
	}
	s.config.BackupRepositoryID = strings.Repeat("a", 64)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	credential := webauthn.Credential{ID: []byte("credential"), PublicKey: []byte("synthetic-begin-only")}
	data, _ := json.Marshal(credential)
	if _, err := s.Store.DB.Exec("INSERT INTO credentials(id,device_id,data) VALUES(?,'device',?)", credential.ID, string(data)); err != nil {
		t.Fatal(err)
	}
	send := func(body, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.URL.Scheme = ""
		r.URL.Host = ""
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	body := `{"repositoryId":"` + s.config.BackupRepositoryID + `"}`
	for _, invalid := range []string{`{}`, `{"repositoryId":"foreign"}`, `{"repositoryId":"` + s.config.BackupRepositoryID + `","password":"secret"}`} {
		if got := send(invalid, "backup-request-1234567890"); got.Code == 200 {
			t.Fatal("invalid approval accepted", invalid)
		}
	}
	if got := send(body, ""); got.Code == 200 {
		t.Fatal("missing request key accepted")
	}
	response := send(body, "backup-request-1234567890")
	var result struct {
		Options   protocol.CredentialAssertion `json:"options"`
		Challenge string                       `json:"challengeToken"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Challenge == "" || result.Options.Response.UserVerification != protocol.VerificationRequired {
		t.Fatal(response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("approval cached")
	}
	if err := s.Store.Transaction(context.Background(), state.RequireAdmission); err != nil {
		t.Fatal("begin approval closed admission", err)
	}
}
