package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func TestDownloadRangeBounds(t *testing.T) {
	for _, test := range []struct {
		value            string
		size, start, end int64
		partial, invalid bool
	}{
		{"", 10, 0, 10, false, false},
		{"bytes=2-5", 10, 2, 6, true, false},
		{"bytes=7-", 10, 7, 10, true, false},
		{"bytes=-3", 10, 7, 10, true, false},
		{"bytes=2-9999999999999999999999999999999999", 10, 2, 10, true, false},
		{"bytes=-9999999999999999999999999999999999", 10, 0, 10, true, false},
		{"bytes=0-1,3-4", 10, 0, 10, false, false},
		{"unknown=0-1", 10, 0, 10, false, false},
		{"bytes=10-", 10, 0, 0, false, true},
		{"bytes=9-2", 10, 0, 0, false, true},
		{"bytes=-0", 10, 0, 0, false, true},
		{"bytes=+1-2", 10, 0, 0, false, true},
		{"bytes=0--2", 10, 0, 0, false, true},
		{"bytes=0-", 0, 0, 0, false, true},
		{"bytes=" + strings.Repeat("9", 130) + "-", 10, 0, 0, false, true},
	} {
		start, end, partial, err := downloadRange(test.value, test.size)
		if (err != nil) != test.invalid || start != test.start || end != test.end || partial != test.partial {
			t.Errorf("%q: %d %d %v %v", test.value, start, end, partial, err)
		}
	}
}

type downloadHookBackend struct {
	workload.Backend
	hook func(context.Context) error
}

func (b downloadHookBackend) Call(ctx context.Context, instance string, request guestproto.Request) (guestproto.Response, error) {
	if request.Operation == "download" {
		if err := b.hook(ctx); err != nil {
			return guestproto.Response{}, err
		}
	}
	return b.Backend.Call(ctx, instance, request)
}

func TestAuthenticatedDownloadRangesPreserveIntegrityAndRevocation(t *testing.T) {
	s := testServer(t)
	token := seedSession(t, s, `["files"]`)
	directory := filepath.Join(t.TempDir(), "objects")
	agent, err := guest.New(directory, "files", 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	backend := fileBackend{agent: agent}
	s.Workloads = workload.New(s.Store, backend, 1)
	if _, err := s.Store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", state.Random()); err != nil {
		t.Fatal(err)
	}
	data := []byte(strings.Repeat("0123456789abcdef", guestproto.ChunkSize/4) + "tail")
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	transfer, err := s.Workloads.CreateTransfer(context.Background(), "device", "content.html", int64(len(data)), hash)
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(data); offset += guestproto.ChunkSize {
		chunk := data[offset:min(offset+guestproto.ChunkSize, len(data))]
		chunkHash := sha256.Sum256(chunk)
		if _, err := s.Workloads.Upload(context.Background(), "device", transfer.ID, int64(offset), chunk, hex.EncodeToString(chunkHash[:])); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Workloads.Finalize(context.Background(), "device", transfer.ID); err != nil {
		t.Fatal(err)
	}
	completed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { completed <- struct{}{} }()
		s.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	send := func(method, value, validator string) (*http.Response, []byte, error) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+"/api/v1/files/"+transfer.ID+"/download", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Host = "localhost:8787"
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: token})
		if value != "" {
			r.Header.Set("Range", value)
		}
		if validator != "" {
			r.Header.Set("If-Range", validator)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		// Flushed headers and short bodies can reach the client before the
		// handler returns. Finish it before changing fixture authority/backend.
		select {
		case <-completed:
		case <-time.After(10 * time.Second):
			t.Fatal("download handler did not finish")
		}
		return response, body, readErr
	}
	etag := `"` + hash + `"`
	for _, test := range []struct {
		value, validator string
		start, end       int
		status           int
	}{
		{"", "", 0, len(data), 200},
		{"bytes=7-", etag, 7, len(data), 206},
		{"bytes=3-19", etag, 3, 20, 206},
		{"bytes=-11", etag, len(data) - 11, len(data), 206},
		{"bytes=7-", `"stale"`, 0, len(data), 200},
		{"bytes=7-", "W/" + etag, 0, len(data), 200},
		{"bytes=7-", "Wed, 21 Oct 2015 07:28:00 GMT", 0, len(data), 200},
		{"unknown=7-", "", 0, len(data), 200},
		{"bytes=0-1,4-5", "", 0, len(data), 200},
	} {
		response, body, err := send("GET", test.value, test.validator)
		if err != nil || response.StatusCode != test.status || string(body) != string(data[test.start:test.end]) {
			t.Fatal("range download", test.value, response.StatusCode, len(body), err)
		}
		if response.Header.Get("ETag") != etag || response.Header.Get("Accept-Ranges") != "bytes" || response.Header.Get("Content-Type") != "application/octet-stream" || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") || !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox") {
			t.Fatal("range lost identity or active-content protections")
		}
		if test.status == 206 && response.Header.Get("Content-Range") != "bytes "+strconv.Itoa(test.start)+"-"+strconv.Itoa(test.end-1)+"/"+strconv.Itoa(len(data)) {
			t.Fatal("incorrect content range", response.Header)
		}
	}
	response, body, err := send("HEAD", "bytes=1-2", etag)
	if err != nil || response.StatusCode != 200 || len(body) != 0 || response.ContentLength != int64(len(data)) || response.Header.Get("Content-Range") != "" {
		t.Fatal("HEAD did not describe full representation", response.StatusCode, err)
	}
	response, _, err = send("GET", "bytes="+strconv.Itoa(len(data))+"-", etag)
	if err != nil || response.StatusCode != 416 || response.Header.Get("Content-Range") != "bytes */"+strconv.Itoa(len(data)) {
		t.Fatal("unsatisfiable range", response.StatusCode, err)
	}
	originalToken := token
	token = ""
	response, _, err = send("GET", "bytes=0-10", etag)
	if err != nil || response.StatusCode != 401 || response.Header.Get("ETag") != "" || response.Header.Get("Content-Range") != "" {
		t.Fatal("range exposed metadata without authentication", response.StatusCode, err)
	}
	token = originalToken
	reads := 0
	s.Workloads.Backend = downloadHookBackend{Backend: backend, hook: func(_ context.Context) error {
		reads++
		if reads == 2 {
			_, err := s.Store.DB.Exec(`UPDATE devices SET capabilities='["ai"]' WHERE id='device'`)
			return err
		}
		return nil
	}}
	response, body, err = send("GET", "bytes=0-10", etag)
	if err == nil || len(body) != 0 {
		t.Fatal("revoked Files capability completed a range", response.StatusCode, len(body), err)
	}
	s.Workloads.Backend = backend
	if _, err := s.Store.DB.Exec(`UPDATE devices SET capabilities='["files"]' WHERE id='device'`); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, transfer.ID+".blob"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("X"), int64(len(data)-1))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	response, body, err = send("GET", "bytes=0-10", etag)
	if err == nil || len(body) != 0 {
		t.Fatal("corruption outside requested range completed a response", response.StatusCode, len(body), err)
	}
}
