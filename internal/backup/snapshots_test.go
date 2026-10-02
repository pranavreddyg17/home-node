package backup

import (
	"strings"
	"testing"
	"time"
)

func TestSnapshotInventoryRequiresBoundedTaggedUnambiguousCandidates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := strings.Repeat("a", 64)
	record := `{"id":"` + id + `","time":"2025-12-31T00:00:00Z","tags":["homenode-v1"],"paths":["/private/source"],"hostname":"private-host"}`
	references, err := parseSnapshotInventory([]byte("["+record+"]"), now)
	if err != nil || len(references) != 1 || references[0].ID != id {
		t.Fatal(references, err)
	}
	if refs, err := parseSnapshotInventory([]byte("[]"), now); err != nil || refs == nil || len(refs) != 0 {
		t.Fatal("empty inventory refused", refs, err)
	}
	for _, data := range []string{
		"null", "{}", "[null]", "[" + record + "," + record + "]", "[" + record + "] trailing",
		"[" + strings.Replace(record, `"id":`, `"ID":`, 1) + "]",
		"[" + strings.Replace(record, `"time":`, `"TIME":`, 1) + "]",
		"[" + strings.Replace(record, `"homenode-v1"`, `"other"`, 1) + "]",
		"[" + strings.Replace(record, "2025-12-31T00:00:00Z", "2026-01-02T00:00:00Z", 1) + "]",
		"[" + strings.Replace(record, `"id":"`+id+`"`, `"id":null`, 1) + "]",
		"[" + record + "]" + string([]byte{0xff}), strings.Repeat(" ", maxSnapshotInventoryBytes+1),
	} {
		if refs, err := parseSnapshotInventory([]byte(data), now); err == nil || refs != nil {
			t.Fatal("invalid inventory admitted", data, refs)
		}
	}
}
