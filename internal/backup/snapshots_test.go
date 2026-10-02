package backup

import (
	"fmt"
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

func TestSnapshotInventoryEntryLimitRejectsWholeResult(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]string, maxSnapshotInventoryEntries+1)
	for index := range records {
		records[index] = fmt.Sprintf(`{"id":"%064x","time":"2025-12-31T00:00:00Z","tags":["homenode-v1"]}`, index)
	}
	allowed := "[" + strings.Join(records[:maxSnapshotInventoryEntries], ",") + "]"
	if entries, err := parseSnapshotInventory([]byte(allowed), now); err != nil || len(entries) != maxSnapshotInventoryEntries {
		t.Fatal("bounded inventory refused", len(entries), err)
	}
	excess := "[" + strings.Join(records, ",") + "]"
	if entries, err := parseSnapshotInventory([]byte(excess), now); err == nil || entries != nil {
		t.Fatal("oversized inventory partially admitted", len(entries), err)
	}
}
