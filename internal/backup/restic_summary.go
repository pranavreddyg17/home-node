package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// parseSnapshotSummary admits one unambiguous successful restic summary while
// allowing version-dependent progress/statistics fields. Process success and
// recovery-set validation must be checked separately by the caller.
func parseSnapshotSummary(data []byte) (string, error) {
	if len(data) == 0 || len(data) > 32768 || !utf8.Valid(data) {
		return "", ErrRepository
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	snapshot := ""
	for {
		var record json.RawMessage
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", ErrRepository
		}
		record = bytes.TrimSpace(record)
		if len(record) == 0 || record[0] != '{' {
			return "", ErrRepository
		}
		if err = uniqueJSON(json.NewDecoder(bytes.NewReader(record)), 0); err != nil {
			return "", ErrRepository
		}
		var summary struct {
			Type string `json:"message_type"`
			ID   string `json:"snapshot_id"`
		}
		if err = json.Unmarshal(record, &summary); err != nil || summary.Type == "" {
			return "", ErrRepository
		}
		if summary.Type == "summary" {
			if snapshot != "" || !repositoryPattern.MatchString(summary.ID) {
				return "", ErrRepository
			}
			snapshot = summary.ID
		} else if summary.ID != "" {
			return "", ErrRepository
		}
	}
	if snapshot == "" {
		return "", ErrRepository
	}
	return snapshot, nil
}
