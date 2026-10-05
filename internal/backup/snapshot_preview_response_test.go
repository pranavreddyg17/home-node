package backup

import (
	"bytes"
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
	"time"
)

func TestPreviewResponseRequiresExactSelectionAndMetadataClassification(t *testing.T) {
	manifest, policy, _ := manifestFixture()
	snapshot, request := state.Hash("selected"), state.Random()
	preview, err := previewSnapshotManifest(snapshot, manifest, policy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeSnapshotPreviewResponse(request, preview)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeSnapshotPreviewResponse(raw, request, snapshot); err != nil || got.SnapshotID != snapshot {
		t.Fatal(got, err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(raw, []byte(`"snapshotId":`), []byte(`"SnapshotId":`), 1),
		bytes.Replace(raw, []byte(`"metadata-compatible"`), []byte(`"restore-tested"`), 1),
		bytes.Replace(raw, []byte(`"workload":`), []byte(`"path":"/host/private","workload":`), 1),
		append(append([]byte{}, raw...), []byte("{}")...),
	} {
		if got, err := DecodeSnapshotPreviewResponse(invalid, request, snapshot); err == nil || got.Files != nil {
			t.Fatal("invalid preview admitted", got, err)
		}
	}
	if _, err := DecodeSnapshotPreviewResponse(raw, state.Random(), snapshot); err == nil {
		t.Fatal("foreign request admitted")
	}
	if _, err := DecodeSnapshotPreviewResponse(raw, request, state.Hash("foreign")); err == nil {
		t.Fatal("foreign selected snapshot admitted")
	}
}
