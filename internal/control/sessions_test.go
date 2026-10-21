package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestActiveSessionInventoryFiltersAndRedacts(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["admin"]`)
	now := time.Now().Unix()
	for _, device := range []struct {
		id      string
		revoked any
	}{{"paired", nil}, {"revoked", now}} {
		if _, err := s.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at,revoked_at) VALUES(?,?,?, ?,?)", device.id, device.id, `["files"]`, now, device.revoked); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range []struct {
		token, device            string
		epoch, lastSeen, expires int64
	}{
		{"private-live-token", "paired", 1, now, now + 1000},
		{"private-expired-token", "paired", 1, now, now - 1},
		{"private-idle-token", "paired", 1, now - 1800, now + 1000},
		{"private-old-epoch-token", "paired", 2, now, now + 1000},
		{"private-revoked-token", "revoked", 1, now, now + 1000},
	} {
		if _, err := s.Store.DB.Exec("INSERT INTO sessions VALUES(?,?,?,?,?,?,?)", state.Hash(session.token), session.device, session.epoch, now, now, session.lastSeen, session.expires); err != nil {
			t.Fatal(err)
		}
	}
	response := request(s, "GET", "http://localhost:8787/api/v1/sessions", "", token, "")
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var summaries []identity.SessionSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summaries); err != nil || len(summaries) != 2 {
		t.Fatal("invalid live inventory", summaries, err)
	}
	current := 0
	for _, summary := range summaries {
		if summary.DeviceID != "device" && summary.DeviceID != "paired" {
			t.Fatal("inactive session exposed", summary)
		}
		if summary.Current {
			current++
			if summary.DeviceID != "device" {
				t.Fatal("wrong current session")
			}
		}
	}
	if current != 1 {
		t.Fatal("current session not uniquely identified")
	}
	for _, secret := range []string{token, state.Hash(token), "private-live-token", state.Hash("private-live-token"), "tokenHash", "token_hash"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("session inventory exposed credential material")
		}
	}
	actor, err := s.Identity.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("UPDATE devices SET capabilities='[\"files\"]' WHERE id='device'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Identity.Sessions(context.Background(), actor); err != identity.ErrDenied {
		t.Fatal("stale administrator read session inventory", err)
	}
	if result := request(s, "GET", "http://localhost:8787/api/v1/sessions", "", token, ""); result.Code != 403 {
		t.Fatal("limited session inventory status", result.Code)
	}
	if result := request(s, "GET", "http://localhost:8787/api/v1/sessions", "", "", ""); result.Code != 401 {
		t.Fatal("unauthenticated inventory status", result.Code)
	}
}
