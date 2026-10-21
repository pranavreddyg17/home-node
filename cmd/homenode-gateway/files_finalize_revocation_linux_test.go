//go:build linux

package main

import (
	"bytes"
	"context"
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

// A second object remains unfinalized across a cold guest restart. This makes
// verification observable without production test hooks or database mutations.
func prepareNativeRevocableFinalization(t *testing.T, request func(string, string, any, int) []byte) string {
	t.Helper()
	const size = 128 << 20
	pattern := make([]byte, 256<<10)
	if _, err := rand.Read(pattern); err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	if n, err := io.Copy(digest, io.LimitReader(&nativePatternReader{pattern: pattern}, size)); err != nil || n != size {
		t.Fatal("finalization fixture hash failed", n, err)
	}
	body := map[string]any{"name": "revoked-finalization.bin", "size": size, "sha256": hex.EncodeToString(digest.Sum(nil))}
	var transfer workload.Transfer
	if json.Unmarshal(request("POST", "/api/v1/transfers", body, 201), &transfer) != nil || transfer.ID == "" {
		t.Fatal("finalization fixture creation failed")
	}
	chunkHash := sha256.Sum256(pattern)
	for offset := int64(0); offset < size; offset += int64(len(pattern)) {
		request("POST", "/api/v1/transfers/"+transfer.ID+"/chunks", map[string]any{"offset": offset, "data": pattern, "sha256": hex.EncodeToString(chunkHash[:])}, 200)
	}
	if json.Unmarshal(request("GET", "/api/v1/transfers/"+transfer.ID, nil, 200), &transfer) != nil || transfer.Offset != size || transfer.State != "uploading" {
		t.Fatal("finalization fixture upload not retained")
	}
	return transfer.ID
}

type nativeRevocableFinalization struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan nativeUploadResult
}

func startNativeRevocableFinalization(t *testing.T, paired *http.Client, origin, id string) *nativeRevocableFinalization {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	f := &nativeRevocableFinalization{id: id, ctx: ctx, cancel: cancel, done: make(chan nativeUploadResult, 1)}
	req, err := http.NewRequestWithContext(ctx, "POST", origin+"/api/v1/transfers/"+id+"/finalize", bytes.NewBufferString("{}"))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: paired.Transport, Jar: paired.Jar}
	go func() { response, err := client.Do(req); f.done <- nativeUploadResult{response, err} }()
	// Observe the public journal state before revocation. Merely starting a
	// request does not establish that verification admission has committed.
	observationContext, cancelObservation := context.WithTimeout(ctx, 3*time.Second)
	defer cancelObservation()
	for observationContext.Err() == nil {
		select {
		case result := <-f.done:
			if result.response != nil {
				result.response.Body.Close()
			}
			cancel()
			t.Fatal("finalization settled before verification/revocation boundary", result.err)
		default:
		}
		observation, _ := http.NewRequestWithContext(observationContext, "GET", origin+"/api/v1/transfers/"+id, nil)
		observation.Header.Set("Origin", origin)
		response, err := client.Do(observation)
		if err != nil {
			cancel()
			t.Fatal("finalization admission observation failed", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
		response.Body.Close()
		var transfer workload.Transfer
		if readErr != nil || len(data) > 4096 || response.StatusCode != 200 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || json.Unmarshal(data, &transfer) != nil {
			cancel()
			t.Fatal("invalid finalization admission observation")
		}
		if transfer.State == "verifying" {
			return f
		}
		if transfer.State != "uploading" {
			cancel()
			t.Fatal("unexpected finalization state before revocation", transfer.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	t.Fatal("native finalization never exposed verification admission")
	return nil
}

func (f *nativeRevocableFinalization) ExpectRevoked(t *testing.T) {
	t.Helper()
	defer f.cancel()
	select {
	case result := <-f.done:
		if result.err != nil {
			t.Fatal("finalization lost explicit authorization denial", result.err)
		}
		defer result.response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(result.response.Body, 4097))
		var refusal struct{ Error struct{ Code string } }
		if err != nil || len(data) > 4096 || result.response.StatusCode != 401 || result.response.TLS == nil || len(result.response.TLS.VerifiedChains) == 0 || json.Unmarshal(data, &refusal) != nil || refusal.Error.Code != "AUTH_REQUIRED" || f.ctx.Err() != nil {
			t.Fatal("revoked verification published or lost explicit denial", result.response.StatusCode, err)
		}
	case <-f.ctx.Done():
		t.Fatal("revoked verification did not settle")
	}
	t.Logf("native HTTPS finalization revocation transfer=%s denied after verification admission; retained publication inspection follows", f.id)
}
