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

// The disposable native Files volume is two GiB. Retain the existing one-GiB
// object and verification-denied upload while filling the remaining capacity.
// All mutation and observation here crosses the public authenticated API.
func nativeHTTPSFilesCapacityRecovery(t *testing.T, owner *http.Client, origin string, request func(string, string, any, int) []byte, files *nativeHTTPSFilesResult) {
	t.Helper()
	if files.Size != 1<<30 {
		t.Fatal("native capacity recovery requires the large Files fixture")
	}
	storage := func() workload.FilesStorage {
		t.Helper()
		var usage workload.FilesStorage
		if err := json.Unmarshal(request("GET", "/api/v1/files/storage", nil, 200), &usage); err != nil || usage.QuotaBytes != workload.StorageQuota {
			t.Fatal("invalid native Files quota observation", err)
		}
		return usage
	}
	before := storage()
	const size = int64(1 << 30)
	pattern := make([]byte, 256<<10)
	if _, err := rand.Read(pattern); err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	if n, err := io.Copy(digest, io.LimitReader(&nativePatternReader{pattern: pattern}, size)); err != nil || n != size {
		t.Fatal("capacity fixture checksum failed", n, err)
	}
	hash := hex.EncodeToString(digest.Sum(nil))
	var transfer workload.Transfer
	if json.Unmarshal(request("POST", "/api/v1/transfers", map[string]any{"name": "native-capacity-probe.bin", "size": size, "sha256": hash}, 201), &transfer) != nil || transfer.ID == "" || transfer.Offset != 0 || transfer.Size != size || transfer.SHA256 != hash || transfer.State != "uploading" {
		t.Fatal("capacity upload admission failed")
	}
	id := transfer.ID
	chunkHash := sha256.Sum256(pattern)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	client := &http.Client{Transport: owner.Transport, Jar: owner.Jar, Timeout: 30 * time.Second}
	refused := false
	for transfer.Offset < size {
		payload, _ := json.Marshal(map[string]any{"offset": transfer.Offset, "data": pattern, "sha256": hex.EncodeToString(chunkHash[:])})
		req, err := http.NewRequestWithContext(ctx, "POST", origin+"/api/v1/transfers/"+id+"/chunks", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("capacity upload lost explicit response", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		response.Body.Close()
		if readErr != nil || len(body) > 64<<10 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("invalid native capacity response", response.StatusCode, readErr)
		}
		if response.StatusCode == 507 {
			var refusal struct{ Error struct{ Code string } }
			if json.Unmarshal(body, &refusal) != nil || refusal.Error.Code != "CAPACITY_UNAVAILABLE" {
				t.Fatal("native capacity refusal lost stable error code")
			}
			refused = true
			break
		}
		var acknowledged workload.Transfer
		if response.StatusCode != 200 || json.Unmarshal(body, &acknowledged) != nil || acknowledged.ID != id || acknowledged.Size != size || acknowledged.SHA256 != hash || acknowledged.State != "uploading" || acknowledged.Offset != transfer.Offset+int64(len(pattern)) {
			t.Fatal("native capacity upload acknowledgement invalid", response.StatusCode)
		}
		transfer = acknowledged
	}
	if !refused || transfer.Offset <= 0 || transfer.Offset >= size {
		t.Fatal("native Files fixture did not reach bounded guest storage pressure", transfer.Offset)
	}
	var observed workload.Transfer
	if json.Unmarshal(request("GET", "/api/v1/transfers/"+id, nil, 200), &observed) != nil || observed.ID != id || observed.Offset != transfer.Offset || observed.State != "uploading" {
		t.Fatal("capacity refusal advanced committed journal progress")
	}
	charged := storage()
	if charged.StoredBytes != before.StoredBytes || charged.ChargedBytes != before.ChargedBytes+size || charged.UploadReservedBytes != before.UploadReservedBytes+size || charged.ReservedFileEntries != before.ReservedFileEntries+1 {
		t.Fatal("capacity refusal released or changed retained reservation")
	}
	for range 2 {
		var cancellation workload.Transfer
		if json.Unmarshal(request("POST", "/api/v1/transfers/"+id+"/cancel", map[string]any{}, 202), &cancellation) != nil || cancellation.ID != id || cancellation.Offset != transfer.Offset || cancellation.State != "cancelling" && cancellation.State != "cancelled" {
			t.Fatal("capacity cancellation lost intent identity")
		}
	}
	settled := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if json.Unmarshal(request("GET", "/api/v1/transfers/"+id, nil, 200), &observed) != nil || observed.ID != id || observed.Offset != transfer.Offset {
			t.Fatal("capacity cleanup lost retained transfer identity")
		}
		if observed.State == "cancelled" {
			settled = true
			break
		}
		if observed.State != "cancelling" {
			t.Fatal("capacity cleanup changed to an unexpected state", observed.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !settled {
		t.Fatal("capacity cancellation did not acknowledge guest cleanup")
	}
	after := storage()
	if after.StoredBytes != before.StoredBytes || after.ChargedBytes != before.ChargedBytes || after.UploadReservedBytes != before.UploadReservedBytes || after.ReservedFileEntries != before.ReservedFileEntries {
		t.Fatal("capacity cleanup did not restore the exact logical reservation")
	}
	t.Logf("native HTTPS capacity recovery transfer=%s refused at offset=%d and cancelled with reservation restored; retained owner integrity follows", id, transfer.Offset)
	files.VerifyOwner()
	t.Log("native HTTPS retained owner file integrity after capacity cleanup passed")
}
