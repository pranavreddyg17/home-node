//go:build linux

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/workload"
)

// Generate bounded windows without retaining a whole large fixture object.
type nativePatternReader struct {
	pattern []byte
	offset  int64
}

type nativeHTTPSFilesResult struct {
	ID, SHA256                      string
	Size                            int64
	Stop, VerifyOwner, RestartGuest func()
}

func (r *nativePatternReader) Read(output []byte) (int, error) {
	total := len(output)
	for len(output) > 0 {
		n := copy(output, r.pattern[int(r.offset%int64(len(r.pattern))):])
		r.offset += int64(n)
		output = output[n:]
	}
	return total, nil
}

// This optional native client requires an actual configured workload backend.
// It never seeds app state or falls back to metadata-only success. Until joined
// to the native VM fixture, compilation alone does not qualify this workflow.
func nativeHTTPSFilesRoundTrip(t *testing.T, client *http.Client, origin string, request func(string, string, any, int) []byte, approve func(string, any) string, approval, idempotency *string) *nativeHTTPSFilesResult {
	t.Helper()
	previousTimeout := client.Timeout
	client.Timeout = 95 * time.Second
	defer func() { client.Timeout = previousTimeout }()
	size := int64(32 << 10)
	switch os.Getenv("HOMENODE_NATIVE_HTTPS_FILES_BYTES") {
	case "", "32768":
	case "1073741824":
		size = 1 << 30
		client.Timeout = 5 * time.Minute
	default:
		t.Fatal("unsupported native HTTPS Files payload size")
	}
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
	expectedHash := func(offset, count int64) string {
		t.Helper()
		digest := sha256.New()
		if n, err := io.Copy(digest, io.LimitReader(&nativePatternReader{data, offset}, count)); err != nil || n != count {
			t.Fatal("fixture payload hash failed", n, err)
		}
		return hex.EncodeToString(digest.Sum(nil))
	}
	hash := expectedHash(0, size)
	*idempotency = key()
	creationKey := *idempotency
	body := map[string]any{"name": "native-https-files.bin", "size": size, "sha256": hash}
	created := request("POST", "/api/v1/transfers", body, 201)
	replayed := request("POST", "/api/v1/transfers", body, 201)
	*idempotency = ""
	var transfer, replay workload.Transfer
	if json.Unmarshal(created, &transfer) != nil || json.Unmarshal(replayed, &replay) != nil || transfer.ID == "" || transfer.ID != replay.ID || transfer.Offset != 0 {
		t.Fatal("Files transfer creation replay changed identity")
	}
	path := "/api/v1/transfers/" + transfer.ID
	window := int64(16 << 10)
	if size > 32<<10 {
		window = 256 << 10
	}
	for offset := int64(0); offset < size; offset += window {
		chunk := make([]byte, min(window, size-offset))
		if _, err := io.ReadFull(&nativePatternReader{data, offset}, chunk); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(chunk)
		payload := map[string]any{"offset": offset, "data": chunk, "sha256": hex.EncodeToString(sum[:])}
		if offset == 0 {
			// Abandon the acknowledgement body, close the transport, then
			// recover the committed offset through a fresh HTTPS request.
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest("POST", origin+path+"/chunks", bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", "application/json")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal("native Files first chunk failed", err)
			}
			valid := response.StatusCode == 200 && response.TLS != nil && len(response.TLS.VerifiedChains) > 0
			response.Body.Close()
			client.CloseIdleConnections()
			if !valid {
				t.Fatal("native Files first chunk did not reach verified HTTPS")
			}
		} else {
			request("POST", path+"/chunks", payload, 200)
		}
		if json.Unmarshal(request("GET", path, nil, 200), &transfer) != nil || transfer.Offset != offset+int64(len(chunk)) {
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
		status, start := http.StatusOK, int64(0)
		contentRange := ""
		if partial {
			req.Header.Set("Range", "bytes=16384-")
			req.Header.Set("If-Range", `"`+hash+`"`)
			status, start = http.StatusPartialContent, 16<<10
			contentRange = fmt.Sprintf("bytes 16384-%d/%d", size-1, size)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("native Files download failed", err)
		}
		defer response.Body.Close()
		actual := sha256.New()
		count, readErr := io.Copy(actual, io.LimitReader(response.Body, size-start+1))
		if readErr != nil || count != size-start || hex.EncodeToString(actual.Sum(nil)) != expectedHash(start, size-start) || response.StatusCode != status || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.Header.Get("ETag") != `"`+hash+`"` || response.Header.Get("X-Content-SHA256") != hash {
			t.Fatal("native Files download bytes, digest, range or verified TLS mismatch", response.StatusCode, readErr)
		}
		if response.ContentLength != size-start || response.Header.Get("Content-Range") != contentRange || response.Header.Get("Accept-Ranges") != "bytes" || response.Header.Get("Content-Type") != "application/octet-stream" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Strict-Transport-Security") == "" || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment;") || !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox") {
			t.Fatal("native Files download range or attachment security contract mismatch")
		}
	}
	download(false)
	interrupted, err := http.NewRequest("GET", origin+"/api/v1/files/"+transfer.ID+"/download", nil)
	if err != nil {
		t.Fatal(err)
	}
	interrupted.Header.Set("Origin", origin)
	response, err := client.Do(interrupted)
	if err != nil {
		t.Fatal("native Files interrupted download failed", err)
	}
	prefix := sha256.New()
	n, readErr := io.CopyN(prefix, response.Body, 16<<10)
	response.Body.Close()
	client.CloseIdleConnections()
	if readErr != nil || n != 16<<10 || response.StatusCode != 200 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.Header.Get("ETag") != `"`+hash+`"` || hex.EncodeToString(prefix.Sum(nil)) != expectedHash(0, 16<<10) {
		t.Fatal("native Files interrupted download prefix mismatch", readErr)
	}
	download(true)
	request("POST", "/api/v1/files/"+transfer.ID+"/actions", map[string]string{"action": "trash"}, 200)
	request("POST", "/api/v1/files/"+transfer.ID+"/actions", map[string]string{"action": "restore"}, 200)
	archiveRequest, err := http.NewRequest("GET", origin+"/api/v1/files/export", nil)
	if err != nil {
		t.Fatal(err)
	}
	archiveRequest.Header.Set("Origin", origin)
	archiveResponse, err := client.Do(archiveRequest)
	if err != nil {
		t.Fatal("native Files export request failed", err)
	}
	defer archiveResponse.Body.Close()
	if archiveResponse.StatusCode != 200 || archiveResponse.TLS == nil || len(archiveResponse.TLS.VerifiedChains) == 0 || archiveResponse.Header.Get("Content-Type") != "application/zip" || archiveResponse.Header.Get("Cache-Control") != "no-store" || archiveResponse.Header.Get("Strict-Transport-Security") == "" {
		t.Fatal("native Files export lacks verified TLS or response protections")
	}
	archive, err := os.CreateTemp(t.TempDir(), "native-files-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	budget := size + 1<<20
	archiveSize, err := io.Copy(archive, io.LimitReader(archiveResponse.Body, budget+1))
	closeErr := archiveResponse.Body.Close()
	if err != nil || closeErr != nil || archiveSize > budget {
		t.Fatal("native Files archive failed or exceeded its disk budget", err, closeErr)
	}
	reader, err := zip.NewReader(archive, archiveSize)
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
			if file.ID != transfer.ID || file.Size != size || file.SHA256 != hash || file.Name != "native-https-files.bin" || file.ArchivePath != "files/00001-native-https-files.bin" || file.TrashUntil != nil {
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
		actual := sha256.New()
		count, err := io.Copy(actual, io.LimitReader(stream, size+1))
		closeErr := stream.Close()
		if found || err != nil || closeErr != nil || count != size || hex.EncodeToString(actual.Sum(nil)) != hash {
			t.Fatal("native Files export content mismatch", err, closeErr)
		}
		found = true
	}
	if !found || !manifestFound {
		t.Fatal("native Files export omitted uploaded object or verified manifest")
	}
	appAction("stop")
	if os.Getenv("HOMENODE_NATIVE_HTTPS_RESTART") == "1" {
		before := request("GET", "/api/v1/session", nil, 200)
		var oldActor, newActor struct{ Device struct{ ID string } }
		if json.Unmarshal(before, &oldActor) != nil || oldActor.Device.ID == "" {
			t.Fatal("restart actor missing")
		}
		fmt.Fprintln(os.Stdout, "NATIVE-RESTART-READY")
		reader := bufio.NewReader(io.LimitReader(os.Stdin, 1024))
		resumed := false
		for range 3 {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal("native restart handshake ended", err)
			}
			line = strings.TrimSpace(line)
			if line == "continue" {
				resumed = true
				break
			}
			if line != "" {
				t.Fatal("unexpected native restart handshake")
			}
		}
		if !resumed {
			t.Fatal("native restart did not resume")
		}
		client.CloseIdleConnections()
		if json.Unmarshal(request("GET", "/api/v1/session", nil, 200), &newActor) != nil || oldActor.Device.ID != newActor.Device.ID {
			t.Fatal("enrolled session did not survive service restart")
		}
		*idempotency = creationKey
		resumedTransfer := request("POST", "/api/v1/transfers", body, 201)
		*idempotency = ""
		var retained workload.Transfer
		if json.Unmarshal(resumedTransfer, &retained) != nil || retained.ID != transfer.ID || retained.Offset != size || retained.State != "ready" || retained.SHA256 != hash {
			t.Fatal("original transfer request did not survive service restart")
		}
		t.Log("native HTTPS all-service restart retained session and original transfer creation identity")
	}
	appAction("start")
	download(false)
	t.Log("native HTTPS Files signed lifecycle, transfer creation replay, chunk observation, verified download/range, Trash/restore, ZIP and retained restart passed")
	t.Logf("native HTTPS Files payload bytes=%d; payload windows <=256 KiB; ZIP disk budget=%d", size, budget)
	// Keep the guest running through paired-device revocation. The caller must
	// verify retained owner access and approve the final stop before recovery.
	return &nativeHTTPSFilesResult{ID: transfer.ID, SHA256: hash, Size: size,
		Stop:         func() { appAction("stop") },
		RestartGuest: func() { appAction("stop"); appAction("start") },
		VerifyOwner: func() {
			previous := client.Timeout
			client.Timeout = 5 * time.Minute
			defer func() { client.Timeout = previous }()
			download(false)
		},
	}
}
