package backup

import (
	"bytes"
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
