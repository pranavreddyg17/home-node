package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func TestBusyDownloadsPreserveOtherDevicesAndManagementCapacity(t *testing.T) {
	s := testServer(t)
	first := seedSession(t, s, `["files"]`)
	seed := func(device string) string {
		t.Helper()
		token := state.Random()
		now := time.Now().Unix()
		if err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'fixture','["files"]',?)`, device, now); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO sessions VALUES(?,?,1,?,?,?,?)", state.Hash(token), device, now, now, now, now+3600)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return token
	}
	second, third := seed("second"), seed("third")
	agent, err := guest.New(filepath.Join(t.TempDir(), "objects"), "files", 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	backend := fileBackend{agent: agent}
	s.Workloads = workload.New(s.Store, backend, 1)
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	data := []byte("bounded download fixture")
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	transfer, err := s.Workloads.CreateTransfer(context.Background(), "device", "bounded.txt", int64(len(data)), hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Workloads.Upload(context.Background(), "device", transfer.ID, 0, data, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Workloads.Finalize(context.Background(), "device", transfer.ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var workers sync.WaitGroup
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); workers.Wait() })
	s.Workloads.Backend = downloadHookBackend{Backend: backend, hook: func(ctx context.Context) error {
		started <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return context.DeadlineExceeded
		}
	}}
	url := "http://localhost:8787/api/v1/files/" + transfer.ID + "/download"
	finished := make(chan *httptest.ResponseRecorder, 4)
	start := func(token string, ctx context.Context) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			r := httptest.NewRequest("GET", url, nil).WithContext(ctx)
			r.URL.Scheme, r.URL.Host = "", ""
			r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
			response := httptest.NewRecorder()
			s.ServeHTTP(response, r)
			finished <- response
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("download did not reach guest")
		}
	}
	busy := func(token string) {
		t.Helper()
		response := request(s, "GET", url, "", token, "")
		if response.Code != 429 || response.Header().Get("Retry-After") != "1" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("download did not refuse excess work", response.Code, response.Body.String())
		}
	}
	cancellation, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	start(first, cancellation)
	start(first, context.Background())
	busy(first)
	cancel()
	select {
	case response := <-finished:
		if response.Code == 200 {
			t.Fatal("cancelled guest read completed a download")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled download retained capacity")
	}
	start(first, context.Background())
	busy(first)
	start(second, context.Background())
	start(second, context.Background())
	busy(third)
	if response := request(s, "GET", "http://localhost:8787/api/v1/healthz", "", "", ""); response.Code != 200 {
		t.Fatal("downloads blocked health checks", response.Code)
	}
	if response := request(s, "GET", "http://localhost:8787/api/v1/session", "", first, ""); response.Code != 200 {
		t.Fatal("downloads blocked session observation", response.Code)
	}
	releaseOnce.Do(func() { close(release) })
	for range 4 {
		select {
		case response := <-finished:
			if response.Code != 200 || response.Body.String() != string(data) {
				t.Fatal("admitted download failed", response.Code, response.Body.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("download did not release capacity")
		}
	}
	if response := request(s, "GET", url, "", third, ""); response.Code != 200 || response.Body.String() != string(data) {
		t.Fatal("released capacity was not reusable", response.Code)
	}
	workers.Wait()
	s.Workloads.Backend = nil
	for range 3 {
		if response := request(s, "GET", url, "", first, ""); response.Code != 503 {
			t.Fatal("failed download leaked an admission slot", response.Code)
		}
	}
	s.Workloads.Backend = backend
	if response := request(s, "GET", url, "", first, ""); response.Code != 200 {
		t.Fatal("capacity did not recover after guest refusal", response.Code)
	}
}
