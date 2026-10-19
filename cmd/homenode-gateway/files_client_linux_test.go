//go:build linux

package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
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
		if partial {
			req.Header.Set("Range", "bytes=16384-")
			req.Header.Set("If-Range", `"`+hash+`"`)
			status, expected = http.StatusPartialContent, data[16<<10:]
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
	found := false
	for _, entry := range reader.File {
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
	if !found {
		t.Fatal("native Files export omitted uploaded object")
	}
	appAction("stop")
	appAction("start")
	download(false)
	appAction("stop")
	t.Log("native HTTPS Files signed lifecycle, transfer creation replay, chunk observation, verified download/range, Trash/restore, ZIP and retained restart passed")
}
