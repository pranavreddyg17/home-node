package backup

import (
	"bytes"
	"fmt"
	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"strings"
	"testing"
	"time"
)

func TestSnapshotPageResponseBindsRequestAndExactMetadata(t *testing.T) {
	request := strings.Repeat("a", 32)
	page := SnapshotPage{Snapshots: []SnapshotReference{{ID: strings.Repeat("b", 64), CreatedAt: time.Now().Add(-time.Hour)}}}
	raw, err := EncodeSnapshotPageResponse(request, page)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSnapshotPageResponse(raw, request)
	if err != nil || len(decoded.Snapshots) != 1 || decoded.Snapshots[0].ID != page.Snapshots[0].ID {
		t.Fatal(decoded, err)
	}
	if _, err := DecodeSnapshotPageResponse(raw, strings.Repeat("c", 32)); err == nil {
		t.Fatal("foreign request reply accepted")
	}
	for _, invalid := range [][]byte{
		bytes.Replace(raw, []byte(`"requestId":`), []byte(`"RequestId":`), 1),
		bytes.Replace(raw, []byte(`"id":`), []byte(`"ID":`), 1),
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(raw, []byte(`"kind":"snapshot-page"`), []byte(`"kind":"backup-complete"`), 1),
		append(append([]byte{}, raw...), []byte(" trailing")...),
	} {
		if _, err := DecodeSnapshotPageResponse(invalid, request); err == nil {
			t.Fatal("ambiguous response admitted", string(invalid))
		}
	}
	page.Next = page.Snapshots[0].ID
	if _, err := EncodeSnapshotPageResponse(request, page); err == nil {
		t.Fatal("short page continuation admitted")
	}
}

func TestFullSnapshotPageResponseFitsActualEnvelope(t *testing.T) {
	request := strings.Repeat("r", 64)
	page := SnapshotPage{Snapshots: make([]SnapshotReference, snapshotPageSize)}
	for index := range page.Snapshots {
		page.Snapshots[index] = SnapshotReference{ID: fmt.Sprintf("%064x", index), CreatedAt: time.Date(2025, 12, 31, 0, 0, 0, 999999999, time.FixedZone("offset", 3600))}
	}
	page.Next = page.Snapshots[len(page.Snapshots)-1].ID
	raw, err := EncodeSnapshotPageResponse(request, page)
	if err != nil || len(raw) > disktransport.MaxPacket {
		t.Fatal("full envelope exceeds packet", len(raw), err)
	}
	decoded, err := DecodeSnapshotPageResponse(raw, request)
	if err != nil || len(decoded.Snapshots) != snapshotPageSize || decoded.Next != page.Next {
		t.Fatal(decoded, err)
	}
}
