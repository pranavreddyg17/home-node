package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

type fileBackend struct{ agent *guest.Agent }

func (b fileBackend) Apply(context.Context, supervisor.Request) (supervisor.Instance, error) {
	return supervisor.Instance{}, nil
}
func (b fileBackend) Call(_ context.Context, _ string, r guestproto.Request) (guestproto.Response, error) {
	return b.agent.Handle(r), nil
}
func TestAuthenticatedTransferAPI(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	agent, err := guest.New(filepath.Join(t.TempDir(), "objects"), "files", 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	s.Workloads = workload.New(s.Store, fileBackend{agent}, 1)
	if _, err = s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',?)", state.Random(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	data := []byte(strings.Repeat("bounded content", 12000))
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	body, _ := json.Marshal(map[string]any{"name": "content.html", "size": len(data), "sha256": digest})
	w := request(s, "POST", "http://localhost:8787/api/v1/transfers", string(body), token, s.config.Origin)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var transfer workload.Transfer
	if err = json.Unmarshal(w.Body.Bytes(), &transfer); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"offset": 0, "data": data, "sha256": digest})
	w = request(s, "POST", "http://localhost:8787/api/v1/transfers/"+transfer.ID+"/chunks", string(body), token, s.config.Origin)
	if w.Code != 200 {
		t.Fatalf("chunk: %d %s", w.Code, w.Body.String())
	}
	w = request(s, "POST", "http://localhost:8787/api/v1/transfers/"+transfer.ID+"/finalize", "{}", token, s.config.Origin)
	if w.Code != 200 {
		t.Fatalf("finalize: %d %s", w.Code, w.Body.String())
	}
	w = request(s, "GET", "http://localhost:8787/api/v1/files/"+transfer.ID+"/download", "", token, "")
	if w.Code != 200 || w.Body.String() != string(data) {
		t.Fatalf("download: %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("active content could execute on management origin")
	}
	body, _ = json.Marshal(map[string]any{"name": "discard.txt", "size": len(data), "sha256": digest})
	w = request(s, "POST", "http://localhost:8787/api/v1/transfers", string(body), token, s.config.Origin)
	if w.Code != 201 {
		t.Fatal("create cancellable upload", w.Code)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &transfer); err != nil {
		t.Fatal(err)
	}
	endpoint := "http://localhost:8787/api/v1/transfers/" + transfer.ID + "/cancel"
	w = request(s, "POST", endpoint, "{}", token, "https://attacker.example")
	if w.Code != 403 {
		t.Fatal("cross-origin cancellation", w.Code)
	}
	w = request(s, "POST", endpoint, "{}", token, s.config.Origin)
	if w.Code != 202 {
		t.Fatal("cancel", w.Code, w.Body.String())
	}
	w = request(s, "POST", "http://localhost:8787/api/v1/transfers/"+transfer.ID+"/finalize", "{}", token, s.config.Origin)
	if w.Code != 409 {
		t.Fatal("cancelled upload finalized", w.Code)
	}
	if _, err = s.Store.DB.Exec("UPDATE devices SET capabilities='[\"jobs\"]'"); err != nil {
		t.Fatal(err)
	}
	w = request(s, http.MethodGet, "http://localhost:8787/api/v1/files", "", token, "")
	if w.Code != 403 {
		t.Fatal("jobs-only device read private files")
	}
	w = request(s, "POST", endpoint, "{}", token, s.config.Origin)
	if w.Code != 403 {
		t.Fatal("jobs-only device cancelled file upload", w.Code)
	}
}
