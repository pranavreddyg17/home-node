package backup

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// SnapshotPageRequest describes metadata selection only. DeviceID is a claimed
// identity until independently authorized by the management service. This type
// is deliberately not admitted by the launch/cleanup worker entry point.
type SnapshotPageRequest struct {
	Version   int    `json:"version"`
	Kind      string `json:"kind"`
	RequestID string `json:"requestId"`
	DeviceID  string `json:"deviceId"`
	Cursor    string `json:"cursor"`
}

func (r SnapshotPageRequest) valid() bool {
	return r.Version == 4 && r.Kind == "snapshot-page" && guestproto.ValidID(r.RequestID) && guestproto.ValidID(r.DeviceID) && (r.Cursor == "" || repositoryPattern.MatchString(r.Cursor))
}

func EncodeSnapshotPageRequest(request SnapshotPageRequest) ([]byte, error) {
	if !request.valid() {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > MaxDispatchBytes {
		return nil, ErrManifest
	}
	return raw, nil
}

func DecodeSnapshotPageRequest(raw []byte) (SnapshotPageRequest, error) {
	if len(raw) == 0 || len(raw) > MaxDispatchBytes || !utf8.Valid(raw) {
		return SnapshotPageRequest{}, ErrManifest
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
		return SnapshotPageRequest{}, ErrManifest
	}
	if _, err := exactManifestObject(raw, []string{"version", "kind", "requestId", "deviceId", "cursor"}, ""); err != nil {
		return SnapshotPageRequest{}, ErrManifest
	}
	var request SnapshotPageRequest
	if json.Unmarshal(raw, &request) != nil || !request.valid() {
		return SnapshotPageRequest{}, ErrManifest
	}
	return request, nil
}
