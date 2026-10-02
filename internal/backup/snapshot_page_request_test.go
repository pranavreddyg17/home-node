package backup

import (
	"bytes"
	"strings"
	"testing"
)

func TestSnapshotPageRequestIsExactAndCannotLaunchMaintenance(t *testing.T) {
	request := SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: strings.Repeat("a", 32), DeviceID: strings.Repeat("b", 32)}
	raw, err := EncodeSnapshotPageRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSnapshotPageRequest(raw)
	if err != nil || decoded != request {
		t.Fatal(decoded, err)
	}
	if _, err := DecodeWorkerRequest(raw); err == nil {
		t.Fatal("metadata request admitted by maintenance worker")
	}
	for _, invalid := range [][]byte{
		bytes.Replace(raw, []byte(`"deviceId":`), []byte(`"DeviceId":`), 1),
		bytes.Replace(raw, []byte(`"version":4`), []byte(`"version":4,"version":4`), 1),
		bytes.Replace(raw, []byte(`"cursor":""`), []byte(`"cursor":null`), 1),
		bytes.Replace(raw, []byte(`"cursor":""`), []byte(`"cursor":"/host/path"`), 1),
		bytes.Replace(raw, []byte(`"cursor":""`), []byte(`"cursor":"","runtimeToken":"owned"`), 1),
		append(append([]byte{}, raw...), []byte(" trailing")...),
	} {
		if _, err := DecodeSnapshotPageRequest(invalid); err == nil {
			t.Fatal("invalid request admitted", string(invalid))
		}
	}
	request.Cursor = strings.Repeat("c", 64)
	if _, err := EncodeSnapshotPageRequest(request); err != nil {
		t.Fatal("full-ID cursor refused", err)
	}
	request.Version = 2
	if _, err := EncodeSnapshotPageRequest(request); err == nil {
		t.Fatal("maintenance request version admitted")
	}
}
