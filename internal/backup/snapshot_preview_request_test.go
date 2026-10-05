package backup

import (
	"bytes"
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
)

func TestSnapshotPreviewRequestCannotGrantListOrMaintenanceAuthority(t *testing.T) {
	request := SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: state.Random(), DeviceID: state.Random(), SnapshotID: state.Hash("selected")}
	raw, err := EncodeSnapshotPreviewRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeSnapshotPreviewRequest(raw)
	if err != nil || got != request {
		t.Fatal(got, err)
	}
	if _, err := DecodeSnapshotPageRequest(raw); err == nil {
		t.Fatal("preview request accepted as list")
	}
	if _, err := DecodeWorkerRequest(raw); err == nil {
		t.Fatal("preview request accepted as maintenance")
	}
	for _, invalid := range [][]byte{
		bytes.Replace(raw, []byte(`"snapshotId":`), []byte(`"SnapshotId":`), 1),
		bytes.Replace(raw, []byte(`"version":5`), []byte(`"version":5,"version":5`), 1),
		bytes.Replace(raw, []byte(request.SnapshotID), []byte("latest"), 1),
		bytes.Replace(raw, []byte(`"snapshotId":"`+request.SnapshotID+`"`), []byte(`"snapshotId":null`), 1),
		bytes.Replace(raw, []byte(`"kind":"snapshot-preview"`), []byte(`"kind":"snapshot-preview","runtimeToken":"`+strings.Repeat("a", 32)+`"`), 1),
		append(append([]byte{}, raw...), []byte("{}")...),
	} {
		if got, err := DecodeSnapshotPreviewRequest(invalid); err == nil || got != (SnapshotPreviewRequest{}) {
			t.Fatal("invalid preview selection admitted", got, err)
		}
	}
	request.SnapshotID = ""
	if _, err := EncodeSnapshotPreviewRequest(request); err == nil {
		t.Fatal("empty selected snapshot admitted")
	}
}
