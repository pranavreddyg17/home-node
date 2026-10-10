package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func TestAuthenticatedUploadCapacityRefusalAndResume(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	dir := filepath.Join(t.TempDir(), "objects")
	agent, err := guest.New(dir, "files", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	s.Workloads = workload.New(s.Store, fileBackend{agent}, 1)
	if _, err = s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',?)", state.Random(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, state.Random()+".blob")
	file, err := os.OpenFile(blocker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate((1 << 30) - (64 << 20) - 4)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	data := []byte("resume this upload after freeing capacity")
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	body, _ := json.Marshal(map[string]any{"name": "resume.txt", "size": len(data), "sha256": digest})
	w := request(s, "POST", "http://localhost:8787/api/v1/transfers", string(body), token, s.config.Origin)
	if w.Code != http.StatusCreated {
		t.Fatal("create", w.Code, w.Body.String())
	}
	var transfer workload.Transfer
	if err := json.Unmarshal(w.Body.Bytes(), &transfer); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"offset": 0, "data": data, "sha256": digest})
	endpoint := "http://localhost:8787/api/v1/transfers/" + transfer.ID + "/chunks"
	w = request(s, "POST", endpoint, string(body), token, s.config.Origin)
	var failure struct {
		Error struct{ Code string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil || w.Code != http.StatusInsufficientStorage || failure.Error.Code != "CAPACITY_UNAVAILABLE" {
		t.Fatal("capacity classification lost at authenticated API", w.Code, w.Body.String(), err)
	}
	w = request(s, "GET", "http://localhost:8787/api/v1/transfers/"+transfer.ID, "", token, "")
	if err := json.Unmarshal(w.Body.Bytes(), &transfer); err != nil || w.Code != http.StatusOK || transfer.Offset != 0 || transfer.State != "uploading" {
		t.Fatal("capacity refusal advanced upload", w.Code, transfer, err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	w = request(s, "POST", endpoint, string(body), token, s.config.Origin)
	if err := json.Unmarshal(w.Body.Bytes(), &transfer); err != nil || w.Code != http.StatusOK || transfer.Offset != int64(len(data)) {
		t.Fatal("authenticated capacity recovery failed", w.Code, transfer, err)
	}
}
