package control

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func exportFixture(t *testing.T) (*Server, string) {
	t.Helper()
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	agent, err := guest.New(filepath.Join(t.TempDir(), "objects"), "files", 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Close() })
	s.Workloads = workload.New(s.Store, fileBackend{agent}, 1)
	if _, err = s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	return s, token
}

func exportFile(t *testing.T, s *Server, name string, data []byte) workload.File {
	t.Helper()
	ctx := context.Background()
	hash := sha256.Sum256(data)
	transfer, err := s.Workloads.CreateTransfer(ctx, "device", name, int64(len(data)), hex.EncodeToString(hash[:]))
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(data); {
		end := min(offset+guestproto.ChunkSize, len(data))
		digest := sha256.Sum256(data[offset:end])
		if _, err = s.Workloads.Upload(ctx, "device", transfer.ID, int64(offset), data[offset:end], hex.EncodeToString(digest[:])); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	file, err := s.Workloads.Finalize(ctx, "device", transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestSavedFilesArchiveHasVerifiedManifestAndSafeUniqueNames(t *testing.T) {
	s, token := exportFixture(t)
	data := bytes.Repeat([]byte("export fixture"), 30000)
	first := exportFile(t, s, "CON:notes?.txt", data)
	second := exportFile(t, s, "CON:notes?.txt", []byte{})
	trashed := exportFile(t, s, "trash.txt", []byte("excluded"))
	if err := s.Workloads.ChangeFile(context.Background(), "device", trashed.ID, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "http://localhost:8787/api/v1/files/export", "", "", ""); w.Code != 401 {
		t.Fatal("export lacked authentication", w.Code)
	}
	w := request(s, "GET", "http://localhost:8787/api/v1/files/export", "", token, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal(w.Code, w.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[file.Name] = body
	}
	var manifest struct {
		Schema int    `json:"schema"`
		Scope  string `json:"scope"`
		Files  []struct {
			workload.File
			ArchivePath string `json:"archivePath"`
		} `json:"files"`
	}
	if err = json.Unmarshal(contents["manifest.json"], &manifest); err != nil || manifest.Schema != 1 || manifest.Scope != "saved-files-excluding-trash" || len(manifest.Files) != 2 || len(contents) != 3 {
		t.Fatal("incomplete manifest", manifest, err)
	}
	for _, file := range manifest.Files {
		expected := data
		if file.ID == second.ID {
			expected = []byte{}
		} else if file.ID != first.ID {
			t.Fatal("unexpected file in export", file.ID)
		}
		actual, exists := contents[file.ArchivePath]
		digest := sha256.Sum256(actual)
		if !exists || !bytes.Equal(actual, expected) || hex.EncodeToString(digest[:]) != file.SHA256 || file.Name != "CON:notes?.txt" {
			t.Fatal("export content or metadata mismatch", file.ID)
		}
		if path.Clean(file.ArchivePath) != file.ArchivePath || !strings.HasPrefix(file.ArchivePath, "files/") || strings.ContainsAny(path.Base(file.ArchivePath), `<>:"/\|?*`) {
			t.Fatal("unsafe export path", file.ArchivePath)
		}
	}
}

type exportFailureBackend struct {
	workload.Backend
	reads   int
	corrupt bool
	revoke  func()
}

func (b *exportFailureBackend) Call(ctx context.Context, instance string, r guestproto.Request) (guestproto.Response, error) {
	response, err := b.Backend.Call(ctx, instance, r)
	if r.Operation == "download" {
		b.reads++
		if b.reads == 2 {
			if b.revoke != nil {
				b.revoke()
			}
			if b.corrupt && len(response.Data) > 0 {
				response.Data[0] ^= 1
				digest := sha256.Sum256(response.Data)
				response.SHA256 = hex.EncodeToString(digest[:])
			}
		}
	}
	return response, err
}

func TestArchiveCorruptionOrRevocationAbortsWithoutCompletedZIP(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		s, token := exportFixture(t)
		exportFile(t, s, "large.bin", bytes.Repeat([]byte("x"), 2*guestproto.ChunkSize+7))
		backend := &exportFailureBackend{Backend: s.Workloads.Backend, corrupt: corrupt}
		if !corrupt {
			backend.revoke = func() {
				if _, err := s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id='device'"); err != nil {
					t.Error(err)
				}
			}
		}
		s.Workloads.Backend = backend
		done := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); s.ServeHTTP(w, r) }))
		t.Cleanup(server.Close)
		client := server.Client()
		client.Timeout = 5 * time.Second
		r, err := http.NewRequest("GET", server.URL+"/api/v1/files/export", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Host = "localhost:8787"
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
		response, err := client.Do(r)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil {
				t.Fatal("failed archive completed HTTP successfully", corrupt, len(body))
			}
			if _, zipErr := zip.NewReader(bytes.NewReader(body), int64(len(body))); zipErr == nil {
				t.Fatal("failed archive retained a complete central directory")
			}
		}
		<-done
		server.Close()
		if !s.downloads.acquire("device") || !s.downloads.acquire("device") {
			t.Fatal("aborted export leaked download capacity")
		}
		s.downloads.release("device")
		s.downloads.release("device")
	}
}

func TestArchiveRefusesOversizedMetadataInsteadOfTruncating(t *testing.T) {
	for _, oversizedCount := range []bool{false, true} {
		s := testServer(t)
		token := seedSession(t, s, `["files"]`)
		count, size := 13, int64(workload.MaxFileBytes)
		if oversizedCount {
			count, size = 10001, 0
		}
		digest := sha256.Sum256(nil)
		err := s.Store.Transaction(context.Background(), func(tx *sql.Tx) error {
			statement, err := tx.Prepare("INSERT INTO files(id,name,size,sha256,created_at,trash_until) VALUES(?,'metadata fixture',?,?,1,NULL)")
			if err != nil {
				return err
			}
			defer statement.Close()
			for i := 0; i < count; i++ {
				if _, err = statement.Exec(state.Random(), size, hex.EncodeToString(digest[:])); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		w := request(s, "GET", "http://localhost:8787/api/v1/files/export", "", token, "")
		if w.Code != 507 || !strings.Contains(w.Body.String(), "EXPORT_LIMIT") || w.Header().Get("Content-Type") == "application/zip" {
			t.Fatal("oversized archive silently truncated", oversizedCount, w.Code, w.Body.String())
		}
	}
}
