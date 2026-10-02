package backup

import (
	"bytes"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type snapshotPageResponse struct {
	Version   int          `json:"version"`
	Kind      string       `json:"kind"`
	RequestID string       `json:"requestId"`
	Page      SnapshotPage `json:"page"`
}

func validSnapshotPage(page SnapshotPage, now time.Time) bool {
	if page.Snapshots == nil || len(page.Snapshots) > snapshotPageSize {
		return false
	}
	seen := map[string]bool{}
	for index, snapshot := range page.Snapshots {
		if !repositoryPattern.MatchString(snapshot.ID) || seen[snapshot.ID] || snapshot.CreatedAt.IsZero() || snapshot.CreatedAt.After(now.Add(5*time.Minute)) {
			return false
		}
		seen[snapshot.ID] = true
		if index > 0 {
			prior := page.Snapshots[index-1]
			if prior.CreatedAt.Before(snapshot.CreatedAt) || (prior.CreatedAt.Equal(snapshot.CreatedAt) && prior.ID >= snapshot.ID) {
				return false
			}
		}
	}
	return page.Next == "" || (len(page.Snapshots) == snapshotPageSize && page.Next == page.Snapshots[len(page.Snapshots)-1].ID)
}

// EncodeSnapshotPageResponse carries candidate metadata only. Peer authentication
// and request authorization must be supplied by the dedicated worker transport.
func EncodeSnapshotPageResponse(requestID string, page SnapshotPage) ([]byte, error) {
	if !guestproto.ValidID(requestID) || !validSnapshotPage(page, time.Now()) {
		return nil, ErrManifest
	}
	raw, err := json.Marshal(snapshotPageResponse{Version: 1, Kind: "snapshot-page", RequestID: requestID, Page: page})
	if err != nil || len(raw) > disktransport.MaxPacket {
		return nil, ErrManifest
	}
	return raw, nil
}

func DecodeSnapshotPageResponse(raw []byte, requestID string) (SnapshotPage, error) {
	if !guestproto.ValidID(requestID) || len(raw) == 0 || len(raw) > disktransport.MaxPacket || !utf8.Valid(raw) {
		return SnapshotPage{}, ErrManifest
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
		return SnapshotPage{}, ErrManifest
	}
	fields, err := exactManifestObject(raw, []string{"version", "kind", "requestId", "page"}, "")
	if err != nil {
		return SnapshotPage{}, ErrManifest
	}
	pageFields, err := exactManifestObject(fields["page"], []string{"snapshots", "next"}, "")
	if err != nil {
		return SnapshotPage{}, ErrManifest
	}
	var snapshots []json.RawMessage
	if json.Unmarshal(pageFields["snapshots"], &snapshots) != nil || snapshots == nil || len(snapshots) > snapshotPageSize {
		return SnapshotPage{}, ErrManifest
	}
	for _, snapshot := range snapshots {
		if _, err := exactManifestObject(snapshot, []string{"id", "createdAt"}, ""); err != nil {
			return SnapshotPage{}, ErrManifest
		}
	}
	var response snapshotPageResponse
	if json.Unmarshal(raw, &response) != nil || response.Version != 1 || response.Kind != "snapshot-page" || response.RequestID != requestID || !validSnapshotPage(response.Page, time.Now()) {
		return SnapshotPage{}, ErrManifest
	}
	return response.Page, nil
}
