package backup

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// SnapshotPreviewRequest selects compatibility inspection only. DeviceID is a claimed
// identity until independently authorized by the management service. This type
// is deliberately not admitted by the launch/cleanup worker entry point.
type SnapshotPreviewRequest struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	RequestID  string `json:"requestId"`
	DeviceID   string `json:"deviceId"`
	SnapshotID string `json:"snapshotId"`
}

func (r SnapshotPreviewRequest) valid() bool {
	return r.Version == 5 && r.Kind == "snapshot-preview" && guestproto.ValidID(r.RequestID) && guestproto.ValidID(r.DeviceID) && repositoryPattern.MatchString(r.SnapshotID)
}

func EncodeSnapshotPreviewRequest(request SnapshotPreviewRequest) ([]byte, error) {
	if !request.valid() {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > MaxDispatchBytes {
		return nil, ErrManifest
	}
	return raw, nil
}

func DecodeSnapshotPreviewRequest(raw []byte) (SnapshotPreviewRequest, error) {
	if len(raw) == 0 || len(raw) > MaxDispatchBytes || !utf8.Valid(raw) {
		return SnapshotPreviewRequest{}, ErrManifest
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
		return SnapshotPreviewRequest{}, ErrManifest
	}
	if _, err := exactManifestObject(raw, []string{"version", "kind", "requestId", "deviceId", "snapshotId"}, ""); err != nil {
		return SnapshotPreviewRequest{}, ErrManifest
	}
	var request SnapshotPreviewRequest
	if json.Unmarshal(raw, &request) != nil || !request.valid() {
		return SnapshotPreviewRequest{}, ErrManifest
	}
	return request, nil
}
