package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

type publicationAuthorityBackend struct {
	fileBackend
	once              sync.Once
	afterVerification func()
}

func (b *publicationAuthorityBackend) Call(ctx context.Context, id string, req guestproto.Request) (guestproto.Response, error) {
	response, err := b.fileBackend.Call(ctx, id, req)
	if req.Operation == "finalize" && err == nil && response.Error == "" {
		b.once.Do(b.afterVerification)
	}
	return response, err
}

func TestFilesPublicationRequiresCurrentAuthorityAfterGuestVerification(t *testing.T) {
	for _, change := range []struct {
		name, sql string
		status    int
	}{
		{"revoked", "UPDATE devices SET revoked_at=1", 401},
		{"logged-out", "DELETE FROM sessions", 401},
		{"expired", "UPDATE sessions SET expires_at=0", 401},
		{"epoch-replaced", "UPDATE identity SET epoch=epoch+1", 401},
		{"capability-removed", "UPDATE devices SET capabilities='[]'", 403},
	} {
		t.Run(change.name, func(t *testing.T) {
			s := testServer(t)
			token := seedSession(t, s, `["files"]`)
			agent, err := guest.New(filepath.Join(t.TempDir(), "objects"), "files", 16<<30)
			if err != nil {
				t.Fatal(err)
			}
			defer agent.Close()
			backend := &publicationAuthorityBackend{fileBackend: fileBackend{agent}, afterVerification: func() {
				if _, err := s.Store.DB.Exec(change.sql); err != nil {
					t.Fatal(err)
				}
			}}
			s.Workloads = workload.New(s.Store, backend, 1)
			if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',?)", state.Random(), time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			data := []byte("verified bytes must not imply authority to publish")
			digest := sha256.Sum256(data)
			hash := hex.EncodeToString(digest[:])
			body, _ := json.Marshal(map[string]any{"name": "retained.bin", "size": len(data), "sha256": hash})
			created := request(s, "POST", s.config.Origin+"/api/v1/transfers", string(body), token, s.config.Origin)
			var transfer workload.Transfer
			if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &transfer) != nil {
				t.Fatal("fixture creation failed", created.Code)
			}
			path := s.config.Origin + "/api/v1/transfers/" + transfer.ID
			body, _ = json.Marshal(map[string]any{"offset": 0, "data": data, "sha256": hash})
			if response := request(s, "POST", path+"/chunks", string(body), token, s.config.Origin); response.Code != 200 {
				t.Fatal("fixture upload failed", response.Code)
			}
			response := request(s, "POST", path+"/finalize", "{}", token, s.config.Origin)
			if response.Code != change.status {
				t.Fatal("withdrawn authority published file", response.Code, response.Body.String())
			}
			var count int
			if err := s.Store.DB.QueryRow("SELECT count(*) FROM files").Scan(&count); err != nil || count != 0 {
				t.Fatal("unauthorized file metadata committed", count, err)
			}
			var phase string
			var offset int64
			if err := s.Store.DB.QueryRow("SELECT state,offset FROM transfers WHERE id=?", transfer.ID).Scan(&phase, &offset); err != nil || phase != "verifying" || offset != int64(len(data)) {
				t.Fatal("uncertain verified transfer was discarded", phase, offset, err)
			}
			usage, err := s.Workloads.FilesStorage(context.Background())
			if err != nil || usage.UploadReservedBytes != int64(len(data)) || usage.ChargedBytes != int64(len(data)) || usage.StoredBytes != 0 {
				t.Fatal("unpublished verified bytes lost their quota reservation", usage, err)
			}
			retained := agent.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "stat", ObjectID: transfer.ID})
			if retained.Error != "" || retained.Size != int64(len(data)) || retained.SHA256 != hash {
				t.Fatal("verified guest bytes were not retained", retained.Error)
			}
			if change.name == "logged-out" {
				// A new authenticated session for the same device can reconcile
				// the retained object without uploading or publishing it twice.
				newToken := state.Random()
				now := time.Now().Unix()
				if _, err := s.Store.DB.Exec("INSERT INTO sessions VALUES(?,'device',1,?,?,?,?)", state.Hash(newToken), now, now, now, now+3600); err != nil {
					t.Fatal(err)
				}
				if response := request(s, "POST", path+"/finalize", "{}", newToken, s.config.Origin); response.Code != 200 {
					t.Fatal("new session could not reconcile verified bytes", response.Code, response.Body.String())
				}
				if response := request(s, "POST", path+"/finalize", "{}", newToken, s.config.Origin); response.Code != 200 {
					t.Fatal("finalization replay failed", response.Code)
				}
				if err := s.Store.DB.QueryRow("SELECT count(*) FROM files").Scan(&count); err != nil || count != 1 {
					t.Fatal("verified publication duplicated metadata", count, err)
				}
			}
		})
	}
}
