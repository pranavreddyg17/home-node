package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
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
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(record, &fields); err != nil {
			return "", ErrRepository
		}
		for name := range fields {
			if (strings.EqualFold(name, "message_type") && name != "message_type") || (strings.EqualFold(name, "snapshot_id") && name != "snapshot_id") {
				return "", ErrRepository
			}
		}
		var messageType, snapshotID string
		if err = json.Unmarshal(fields["message_type"], &messageType); err != nil || messageType == "" {
			return "", ErrRepository
		}
		if raw, present := fields["snapshot_id"]; present {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return "", ErrRepository
			}
			if err = json.Unmarshal(raw, &snapshotID); err != nil {
				return "", ErrRepository
			}
		}
		if messageType == "summary" {
			if snapshot != "" || !repositoryPattern.MatchString(snapshotID) {
				return "", ErrRepository
			}
			snapshot = snapshotID
		} else if snapshotID != "" {
			return "", ErrRepository
		}
	}
	if snapshot == "" {
		return "", ErrRepository
	}
	return snapshot, nil
}
