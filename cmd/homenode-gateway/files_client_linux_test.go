//go:build linux

package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/workload"
)

// This optional native client requires an actual configured workload backend.
// It never seeds app state or falls back to metadata-only success. Until joined
// to the native VM fixture, compilation alone does not qualify this workflow.
func nativeHTTPSFilesRoundTrip(t *testing.T, client *http.Client, origin string, request func(string, string, any, int) []byte, approve func(string, any) string, approval, idempotency *string) {
	t.Helper()
	previousTimeout := client.Timeout
	client.Timeout = 95 * time.Second
	defer func() { client.Timeout = previousTimeout }()
	var configuration struct {
		RuntimeConfigured bool `json:"runtimeConfigured"`
	}
	if err := json.Unmarshal(request("GET", "/api/v1/apps", nil, 200), &configuration); err != nil || !configuration.RuntimeConfigured {
		t.Fatal("native Files workflow requires real configured workload sockets", err)
	}
	var initial []workload.File
	if err := json.Unmarshal(request("GET", "/api/v1/files", nil, 200), &initial); err != nil || len(initial) != 0 {
		t.Fatal("native Files workflow requires empty initial file metadata", err)
	}
	key := func() string {
		value := make([]byte, 24)
		if _, err := rand.Read(value); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(value)
	}
	appAction := func(action string) {
		t.Helper()
		*idempotency = key()
		body := map[string]string{"action": action}
		*approval = approve("/api/v1/apps/files/actions", body)
		data := request("POST", "/api/v1/apps/files/actions", body, 202)
		*approval, *idempotency = "", ""
		var operation workload.Operation
		if err := json.Unmarshal(data, &operation); err != nil || operation.ID == "" {
			t.Fatal("Files app action returned no operation", err)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			if err := json.Unmarshal(request("GET", "/api/v1/operations/"+operation.ID, nil, 200), &operation); err != nil {
				t.Fatal("invalid Files operation observation", err)
			}
			if operation.State == "succeeded" {
				return
			}
			if operation.State != "pending" && operation.State != "executing" {
				t.Fatal("Files app action did not succeed", action, operation.State)
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("Files app action did not settle", action)
	}
	appAction("start")
	data := make([]byte, 32<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	*idempotency = key()
	body := map[string]any{"name": "native-https-files.bin", "size": len(data), "sha256": hash}
	created := request("POST", "/api/v1/transfers", body, 201)
	replayed := request("POST", "/api/v1/transfers", body, 201)
	*idempotency = ""
	var transfer, replay workload.Transfer
	if json.Unmarshal(created, &transfer) != nil || json.Unmarshal(replayed, &replay) != nil || transfer.ID == "" || transfer.ID != replay.ID || transfer.Offset != 0 {
		t.Fatal("Files transfer creation replay changed identity")
	}
	path := "/api/v1/transfers/" + transfer.ID
	for offset := 0; offset < len(data); offset += 16 << 10 {
		chunk := data[offset : offset+(16<<10)]
		sum := sha256.Sum256(chunk)
		payload := map[string]any{"offset": offset, "data": chunk, "sha256": hex.EncodeToString(sum[:])}
		request("POST", path+"/chunks", payload, 200)
		if json.Unmarshal(request("GET", path, nil, 200), &transfer) != nil || transfer.Offset != int64(offset+len(chunk)) {
			t.Fatal("Files committed chunk observation mismatch")
		}
	}
	request("POST", path+"/finalize", map[string]any{}, 200)
	download := func(partial bool) {
		t.Helper()
		req, err := http.NewRequest("GET", origin+"/api/v1/files/"+transfer.ID+"/download", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		status, expected := http.StatusOK, data
		contentRange := ""
		if partial {
			req.Header.Set("Range", "bytes=16384-")
			req.Header.Set("If-Range", `"`+hash+`"`)
			status, expected = http.StatusPartialContent, data[16<<10:]
			contentRange = fmt.Sprintf("bytes 16384-%d/%d", len(data)-1, len(data))
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("native Files download failed", err)
		}
		defer response.Body.Close()
		actual, readErr := io.ReadAll(io.LimitReader(response.Body, int64(len(data)+1)))
		if readErr != nil || response.StatusCode != status || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.Header.Get("ETag") != `"`+hash+`"` || response.Header.Get("X-Content-SHA256") != hash || !bytes.Equal(actual, expected) {
			t.Fatal("native Files download bytes, digest, range or verified TLS mismatch", response.StatusCode, readErr)
		}
		if response.ContentLength != int64(len(expected)) || response.Header.Get("Content-Range") != contentRange || response.Header.Get("Accept-Ranges") != "bytes" || response.Header.Get("Content-Type") != "application/octet-stream" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Strict-Transport-Security") == "" || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment;") || !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox") {
			t.Fatal("native Files download range or attachment security contract mismatch")
		}
	}
	download(false)
	download(true)
	request("POST", "/api/v1/files/"+transfer.ID+"/actions", map[string]string{"action": "trash"}, 200)
	request("POST", "/api/v1/files/"+transfer.ID+"/actions", map[string]string{"action": "restore"}, 200)
	archive := request("GET", "/api/v1/files/export", nil, 200)
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal("native Files export is not a completed ZIP", err)
	}
	if len(reader.File) != 2 {
		t.Fatal("native Files export must contain one object and its manifest")
	}
	found, manifestFound := false, false
	for _, entry := range reader.File {
		if entry.Name == "manifest.json" {
			stream, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			encoded, readErr := io.ReadAll(io.LimitReader(stream, 4097))
			closeErr := stream.Close()
			var manifest struct {
				Schema int
				Scope  string
				Files  []struct {
					workload.File
					ArchivePath string `json:"archivePath"`
				}
			}
			if manifestFound || readErr != nil || closeErr != nil || len(encoded) > 4096 || json.Unmarshal(encoded, &manifest) != nil || manifest.Schema != 1 || manifest.Scope != "saved-files-excluding-trash" || len(manifest.Files) != 1 {
				t.Fatal("native Files export manifest invalid", readErr, closeErr)
			}
			file := manifest.Files[0]
			if file.ID != transfer.ID || file.Size != int64(len(data)) || file.SHA256 != hash || file.Name != "native-https-files.bin" || file.ArchivePath != "files/00001-native-https-files.bin" || file.TrashUntil != nil {
				t.Fatal("native Files export manifest identity or hash mismatch")
			}
			manifestFound = true
			continue
		}
		if entry.Name != "files/00001-native-https-files.bin" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		actual, err := io.ReadAll(io.LimitReader(stream, int64(len(data)+1)))
		closeErr := stream.Close()
		if found || err != nil || closeErr != nil || !bytes.Equal(actual, data) {
			t.Fatal("native Files export content mismatch", err, closeErr)
		}
		found = true
	}
	if !found || !manifestFound {
		t.Fatal("native Files export omitted uploaded object or verified manifest")
	}
	appAction("stop")
	appAction("start")
	download(false)
	appAction("stop")
	t.Log("native HTTPS Files signed lifecycle, transfer creation replay, chunk observation, verified download/range, Trash/restore, ZIP and retained restart passed")
}
