package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pranavreddyg17/home-node/internal/state"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApprovalBindingCoversAuthorityAndCopiesResources(t *testing.T) {
	actor := Session{Device: Device{ID: "owner-device"}, Epoch: 1, TokenHash: strings.Repeat("a", 64)}
	resources := []string{"video", "files"}
	b, err := newApprovalBinding(actor, "app.stop", resources, []byte(`{"action":"stop"}`), 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	resources[0] = "ai"
	if b.Resources[1] != "video" {
		t.Fatal("binding aliased caller resources")
	}
	other, err := newApprovalBinding(actor, "app.stop", []string{"files", "video"}, []byte(`{"action":"stop"}`), 1, 1000)
	if err != nil || other.digest() != b.digest() {
		t.Fatal("resource order changed authority", err)
	}
	changes := []ApprovalBinding{b, b, b, b, b, b, b, b}
	changes[0].Action = "app.start"
	changes[1].Resources = []string{"ai"}
	changes[2].BodySHA256 = strings.Repeat("b", 64)
	changes[3].PolicyGeneration++
	changes[4].Epoch++
	changes[5].ExpiresAt--
	changes[6].DeviceID = "another-device"
	changes[7].SessionHash = strings.Repeat("c", 64)
	for i, c := range changes {
		if c.digest() == b.digest() {
			t.Fatal("authority change not bound", i)
		}
	}
}

func TestApprovalBindingRejectsInvalidOrExpiredAuthority(t *testing.T) {
	actor := Session{Device: Device{ID: "owner-device"}, Epoch: 1, TokenHash: strings.Repeat("a", 64)}
	b, err := newApprovalBinding(actor, "device.revoke", []string{"target"}, nil, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	cases := []ApprovalBinding{b, b, b, b, b, b, b, b, b, b}
	cases[0].Action = "shell.execute"
	cases[1].Resources = []string{"target", "target"}
	cases[2].Resources = []string{"../target"}
	cases[3].PolicyGeneration = 0
	cases[4].Epoch = 0
	cases[5].ExpiresAt = 1000
	cases[6].ExpiresAt = 1121
	cases[7].BodySHA256 = strings.Repeat("A", 64)
	cases[8].SessionHash = "missing"
	cases[9].DeviceID = ""
	for i, c := range cases {
		if c.valid(1000) {
			t.Fatal("invalid binding accepted", i)
		}
	}
	if b.valid(1120) || b.valid(999) {
		t.Fatal("expired binding or rollback accepted")
	}
}

func TestBeginApprovalRestrictsCredentialAndPersistsBinding(t *testing.T) {
	s := testService(t)
	actor, _ := testDevice(t, s, AllCapabilities)
	other, _ := testDevice(t, s, AllCapabilities)
	if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
		t.Fatal(err)
	}
	for _, device := range []string{actor.Device.ID, other.Device.ID} {
		c := webauthn.Credential{ID: []byte(device), PublicKey: []byte("fixture-only")}
		data, _ := json.Marshal(c)
		if _, err := s.Store.DB.Exec("INSERT INTO credentials(id,device_id,data) VALUES(?,?,?)", c.ID, device, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	options, token, b, err := s.BeginApproval(context.Background(), actor, "device.revoke", []string{other.Device.ID}, []byte("body"), 1)
	if err != nil {
		t.Fatal(err)
	}
	assertion := options.(*protocol.CredentialAssertion)
	if len(assertion.Response.AllowedCredentials) != 1 || string(assertion.Response.AllowedCredentials[0].CredentialID) != actor.Device.ID || assertion.Response.UserVerification != protocol.VerificationRequired {
		t.Fatal("ceremony widened authority")
	}
	var payload, kind string
	var expires int64
	if err := s.Store.DB.QueryRow("SELECT kind,payload,expires_at FROM challenges WHERE token_hash=?", state.Hash(token)).Scan(&kind, &payload, &expires); err != nil {
		t.Fatal(err)
	}
	var c challenge
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		t.Fatal(err)
	}
	if kind != "approval" || c.Binding == nil || c.Binding.digest() != b.digest() || expires != b.ExpiresAt || c.Session.Expires.Unix() != expires {
		t.Fatal("binding not persisted")
	}
	for i := 1; i < 20; i++ {
		if _, _, _, err := s.BeginApproval(context.Background(), actor, "device.revoke", []string{other.Device.ID}, nil, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := s.BeginApproval(context.Background(), actor, "device.revoke", []string{other.Device.ID}, nil, 1); err == nil {
		t.Fatal("unbounded ceremonies accepted")
	}
}

func TestFailedApprovalVerificationConsumesCeremonyWithoutGrant(t *testing.T) {
	for _, change := range []string{"malformed assertion", "other session", "changed policy", "expired", "wrong kind"} {
		t.Run(change, func(t *testing.T) {
			s := testService(t)
			actor, _ := testDevice(t, s, AllCapabilities)
			b, err := newApprovalBinding(actor, "device.pair", nil, nil, 1, time.Now().Unix())
			if err != nil {
				t.Fatal(err)
			}
			kind := "approval"
			policy := int64(1)
			switch change {
			case "other session":
				b.SessionHash = strings.Repeat("c", 64)
			case "changed policy":
				policy = 2
			case "expired":
				b.ExpiresAt = time.Now().Unix() - 1
			case "wrong kind":
				kind = "login"
			}
			payload, _ := json.Marshal(challenge{Binding: &b, Issuer: actor.Device.ID})
			token := state.Random()
			if _, err := s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,?,?,?,?)", state.Hash(token), kind, string(payload), actor.Epoch, time.Now().Unix()+120); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				grant, err := s.FinishApproval(context.Background(), actor, token, policy, httptest.NewRequest("POST", "http://localhost:8787", strings.NewReader("{}")))
				if !errors.Is(err, ErrDenied) || grant != "" {
					t.Fatal("invalid verification authorized", grant, err)
				}
			}
			var count int
			if err := s.Store.DB.QueryRow("SELECT count(*) FROM challenges").Scan(&count); err != nil || count != 0 {
				t.Fatal("ceremony reused or grant minted", count, err)
			}
		})
	}
}

func TestApprovalConsumptionCommitsOnceAndRollsBackFailedMutation(t *testing.T) {
	s := testService(t)
	actor, _ := testDevice(t, s, AllCapabilities)
	body := []byte("exact body")
	binding, err := newApprovalBinding(actor, "device.pair", []string{"phone"}, body, 1, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	token := state.Random()
	data, _ := json.Marshal(challenge{Binding: &binding, Issuer: actor.Device.ID})
	if _, err := s.Store.DB.Exec("INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,?,?)", state.Hash(token), string(data), actor.Epoch, binding.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	consume := func(body []byte, policy int64, fn func(*sql.Tx) error) error {
		return s.ConsumeApproval(context.Background(), actor, token, "device.pair", []string{"phone"}, body, policy, fn)
	}
	calls := 0
	failed := errors.New("injected mutation failure")
	if err := consume(body, 1, func(tx *sql.Tx) error {
		calls++
		if _, err := tx.Exec("UPDATE identity SET claimed=1"); err != nil {
			return err
		}
		return failed
	}); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	var claimed bool
	if err := s.Store.DB.QueryRow("SELECT claimed FROM identity").Scan(&claimed); err != nil || claimed {
		t.Fatal("mutation did not roll back", claimed, err)
	}
	for _, change := range []string{"body", "policy"} {
		b, p := body, int64(1)
		if change == "body" {
			b = []byte("changed body")
		} else {
			p = 2
		}
		if err := consume(b, p, func(*sql.Tx) error { calls++; return nil }); !errors.Is(err, ErrDenied) {
			t.Fatal("changed authority accepted", change, err)
		}
	}
	if err := consume(body, 1, func(tx *sql.Tx) error { calls++; _, err := tx.Exec("UPDATE identity SET claimed=1"); return err }); err != nil {
		t.Fatal("retry failed", err)
	}
	if err := consume(body, 1, func(*sql.Tx) error { calls++; return nil }); !errors.Is(err, ErrDenied) {
		t.Fatal("grant replay accepted", err)
	}
	if calls != 2 {
		t.Fatal("denied mutation ran", calls)
	}
}
