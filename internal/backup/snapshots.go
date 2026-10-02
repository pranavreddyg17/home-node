package backup

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSnapshotInventoryBytes = 1 << 20
const maxSnapshotInventoryEntries = 1000

// SnapshotReference identifies a candidate, not a validated recovery set.
// Repository paths, host names and user-controlled tags are not returned.
type SnapshotReference struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

func parseSnapshotInventory(data []byte, now time.Time) ([]SnapshotReference, error) {
	if len(data) == 0 || len(data) > maxSnapshotInventoryBytes || !utf8.Valid(data) {
		return nil, ErrRepository
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return nil, ErrRepository
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil || len(records) > maxSnapshotInventoryEntries {
		return nil, ErrRepository
	}
	result := make([]SnapshotReference, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		for name := range record {
			for _, key := range []string{"id", "time", "tags"} {
				if strings.EqualFold(name, key) && name != key {
					return nil, ErrRepository
				}
			}
		}
		var id string
		var created time.Time
		var tags []string
		if json.Unmarshal(record["id"], &id) != nil || !repositoryPattern.MatchString(id) || seen[id] {
			return nil, ErrRepository
		}
		if json.Unmarshal(record["time"], &created) != nil || created.IsZero() || created.After(now.Add(5*time.Minute)) {
			return nil, ErrRepository
		}
		if json.Unmarshal(record["tags"], &tags) != nil {
			return nil, ErrRepository
		}
		tagged := false
		for _, tag := range tags {
			if tag == "homenode-v1" {
				tagged = true
			}
		}
		if !tagged {
			return nil, ErrRepository
		}
		seen[id] = true
		result = append(result, SnapshotReference{ID: id, CreatedAt: created})
	}
	return result, nil
}
