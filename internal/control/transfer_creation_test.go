package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func TestTransferCreationHTTPIdempotency(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	// Creation reserves metadata without invoking guest operations.
	s.Workloads = workload.New(s.Store, fileBackend{}, 1)
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	key := state.Random()
	send := func(name, requestKey string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"name": name, "size": 3, "sha256": strings.Repeat("a", 64)})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "http://localhost:8787/api/v1/transfers", strings.NewReader(string(body)))
		r.URL.Scheme, r.URL.Host = "", ""
		r.Header.Set("Origin", s.config.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", requestKey)
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
		response := httptest.NewRecorder()
		s.ServeHTTP(response, r)
		return response
	}
	var original string
	for range 2 {
		response := send("original.txt", key)
		var transfer workload.Transfer
		if err := json.Unmarshal(response.Body.Bytes(), &transfer); err != nil || response.Code != 201 {
			t.Fatal("creation/replay failed", response.Code, response.Body.String(), err)
		}
		if original == "" {
			original = transfer.ID
		} else if transfer.ID != original {
			t.Fatal("replay reserved another upload")
		}
	}
	if response := send("changed.txt", key); response.Code != 409 {
		t.Fatal("changed request accepted", response.Code)
	}
	if response := send("bad-key.txt", "short"); response.Code != 400 {
		t.Fatal("invalid key accepted", response.Code)
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT count(*) FROM transfers").Scan(&count); err != nil || count != 1 {
		t.Fatal("creation replay changed reservation count", count, err)
	}
}
