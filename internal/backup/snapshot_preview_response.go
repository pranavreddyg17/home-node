package backup

import (
	"bytes"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type snapshotPreviewResponse struct {
	Version   int             `json:"version"`
	Kind      string          `json:"kind"`
	RequestID string          `json:"requestId"`
	Preview   SnapshotPreview `json:"preview"`
}

func validSnapshotPreview(preview SnapshotPreview, now time.Time) bool {
	if !repositoryPattern.MatchString(preview.SnapshotID) || preview.CreatedAt.IsZero() || preview.CreatedAt.After(now.Add(5*time.Minute)) || !releasePattern.MatchString(preview.Release) || preview.CatalogVersion < 1 || preview.Validation != "metadata-compatible" || len(preview.Files) < 1 || len(preview.Files) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, file := range preview.Files {
		if seen[file.Workload] || file.Bytes <= 0 {
			return false
		}
		seen[file.Workload] = true
		switch file.Workload {
		case "management":
			if file.Bytes > 256<<20 {
				return false
			}
		case "files", "ai":
			if file.Bytes > 512<<30 {
				return false
			}
		default:
			return false
		}
	}
	return seen["management"]
}

func EncodeSnapshotPreviewResponse(requestID string, preview SnapshotPreview) ([]byte, error) {
	if !guestproto.ValidID(requestID) || !validSnapshotPreview(preview, time.Now()) {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(snapshotPreviewResponse{Version: 1, Kind: "snapshot-preview", RequestID: requestID, Preview: preview})
	if err != nil || len(raw) > disktransport.MaxPacket {
		return nil, ErrManifest
	}
	return raw, nil
}

// Correlation is not peer authentication or restore authority.
func DecodeSnapshotPreviewResponse(raw []byte, requestID, snapshotID string) (SnapshotPreview, error) {
	if !guestproto.ValidID(requestID) || !repositoryPattern.MatchString(snapshotID) || len(raw) == 0 || len(raw) > disktransport.MaxPacket || !utf8.Valid(raw) {
		return SnapshotPreview{}, ErrManifest
	}
	if uniqueJSON(json.NewDecoder(bytes.NewReader(raw)), 0) != nil {
		return SnapshotPreview{}, ErrManifest
	}
	fields, err := exactManifestObject(raw, []string{"version", "kind", "requestId", "preview"}, "")
	if err != nil {
		return SnapshotPreview{}, ErrManifest
	}
	previewFields, err := exactManifestObject(fields["preview"], []string{"snapshotId", "createdAt", "release", "catalogVersion", "validation", "files"}, "")
	if err != nil {
		return SnapshotPreview{}, ErrManifest
	}
	var files []json.RawMessage
	if json.Unmarshal(previewFields["files"], &files) != nil || len(files) < 1 || len(files) > 3 {
		return SnapshotPreview{}, ErrManifest
	}
	for _, file := range files {
		if _, err := exactManifestObject(file, []string{"workload", "bytes"}, ""); err != nil {
			return SnapshotPreview{}, ErrManifest
		}
	}
	var response snapshotPreviewResponse
	if json.Unmarshal(raw, &response) != nil || response.Version != 1 || response.Kind != "snapshot-preview" || response.RequestID != requestID || response.Preview.SnapshotID != snapshotID || !validSnapshotPreview(response.Preview, time.Now()) {
		return SnapshotPreview{}, ErrManifest
	}
	return response.Preview, nil
}
