package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

type filesAuthorityBackend struct {
	fileBackend
	calls atomic.Int64
}

func (b *filesAuthorityBackend) Call(ctx context.Context, id string, req guestproto.Request) (guestproto.Response, error) {
	b.calls.Add(1)
	return b.fileBackend.Call(ctx, id, req)
}

type authorityChangeReader struct {
	io.Reader
	once   sync.Once
	change func()
}

func (r *authorityChangeReader) Read(data []byte) (int, error) {
	r.once.Do(r.change)
	return r.Reader.Read(data)
}

func TestFilesMutationsRecheckAuthorityAfterBodyReception(t *testing.T) {
	for _, route := range []string{"create", "chunk", "cancel"} {
		for _, change := range []struct {
			name, sql string
			status    int
		}{
			{"revoked", "UPDATE devices SET revoked_at=1", 401},
			{"logged-out", "DELETE FROM sessions", 401},
			{"capability-removed", "UPDATE devices SET capabilities='[]'", 403},
		} {
			t.Run(route+"/"+change.name, func(t *testing.T) {
				s := testServer(t)
				token := seedSession(t, s, `["files"]`)
				agent, err := guest.New(filepath.Join(t.TempDir(), "objects"), "files", 16<<30)
				if err != nil {
					t.Fatal(err)
				}
				defer agent.Close()
				backend := &filesAuthorityBackend{fileBackend: fileBackend{agent}}
				s.Workloads = workload.New(s.Store, backend, 1)
				if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',?)", state.Random(), time.Now().Unix()); err != nil {
					t.Fatal(err)
				}
				data := []byte("revocation must precede this guest effect")
				sum := sha256.Sum256(data)
				body := map[string]any{"name": "pending.bin", "size": len(data), "sha256": hex.EncodeToString(sum[:])}
				encoded, _ := json.Marshal(body)
				created := request(s, "POST", s.config.Origin+"/api/v1/transfers", string(encoded), token, s.config.Origin)
				var transfer workload.Transfer
				if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &transfer) != nil || transfer.ID == "" {
					t.Fatal("fixture transfer creation failed", created.Code)
				}
				backend.calls.Store(0)
				path := "/api/v1/transfers"
				if route == "chunk" {
					path += "/" + transfer.ID + "/chunks"
					body = map[string]any{"offset": 0, "data": data, "sha256": hex.EncodeToString(sum[:])}
				} else if route == "cancel" {
					path += "/" + transfer.ID + "/cancel"
					body = map[string]any{}
				}
				encoded, _ = json.Marshal(body)
				reader := &authorityChangeReader{Reader: bytes.NewReader(encoded), change: func() {
					if _, err := s.Store.DB.Exec(change.sql); err != nil {
						t.Fatal(err)
					}
				}}
				r := httptest.NewRequest("POST", s.config.Origin+path, reader)
				r.URL.Scheme, r.URL.Host = "", ""
				r.Header.Set("Origin", s.config.Origin)
				r.Header.Set("Content-Type", "application/json")
				r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
				response := httptest.NewRecorder()
				s.ServeHTTP(response, r)
				if response.Code != change.status || backend.calls.Load() != 0 {
					t.Fatal("stale body actor reached Files effects", response.Code, backend.calls.Load())
				}
				var count, offset int64
				var status string
				if err := s.Store.DB.QueryRow("SELECT count(*) FROM transfers").Scan(&count); err != nil || count != 1 {
					t.Fatal("denied body created metadata", count, err)
				}
				if err := s.Store.DB.QueryRow("SELECT offset,state FROM transfers WHERE id=?", transfer.ID).Scan(&offset, &status); err != nil || offset != 0 || status != "uploading" {
					t.Fatal("denied body changed transfer", offset, status, err)
				}
			})
		}
	}
}
