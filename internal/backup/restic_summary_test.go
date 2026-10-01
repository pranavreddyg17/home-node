package backup

import (
	"errors"
	"strings"
	"testing"
)

func TestSnapshotSummaryRequiresUnambiguousIdentity(t *testing.T) {
	id := strings.Repeat("a", 64)
	valid := `{"message_type":"summary","snapshot_id":"` + id + `","total_bytes_processed":42}`
	for _, data := range []string{valid, `{"message_type":"status","percent_done":0.5}` + "\n" + valid} {
		got, err := parseSnapshotSummary([]byte(data))
		if err != nil || got != id {
			t.Fatal(got, err)
		}
	}
	for _, data := range []string{
		"", "null", "[]", "42", valid + valid, valid + "garbage",
		`{"message_type":"summary","message_type":"status","snapshot_id":"` + id + `"}`,
		`{"message_type":"summary","snapshot_id":"` + id + `","snapshot_\u0069d":"` + id + `"}`,
		`{"message_type":"summary","snapshot_id":null}`,
		`{"message_type":"summary","snapshot_id":"ABC"}`,
		`{"message_type":"status","snapshot_id":"` + id + `"}` + valid,
		`{"message_type":"summary","snapshot_id":"` + id + `","stats":{"x":1,"x":2}}`,
		valid + string([]byte{0xff}), strings.Repeat(" ", 32769) + valid,
	} {
		if got, err := parseSnapshotSummary([]byte(data)); got != "" || !errors.Is(err, ErrRepository) {
			t.Fatalf("accepted %q: %q %v", data, got, err)
		}
	}
}
