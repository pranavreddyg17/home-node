package backup

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

func TestSnapshotPagesBoundWireSizeAndRequireExistingCursor(t *testing.T) {
	candidates := make([]SnapshotReference, 52)
	for index := range candidates {
		candidates[index] = SnapshotReference{ID: fmt.Sprintf("%064x", index), CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 999999999, time.FixedZone("offset", 3600))}
	}
	cursor := ""
	visited := []string{}
	for {
		page, err := pageSnapshotCandidates(candidates, cursor)
		if err != nil || len(page.Snapshots) > snapshotPageSize {
			t.Fatal(page, err)
		}
		encoded, err := json.Marshal(page)
		// Leave room for a versioned request-correlated response envelope.
		if err != nil || len(encoded)+256 > disktransport.MaxPacket {
			t.Fatal("page exceeds packet budget", len(encoded), err)
		}
		for _, candidate := range page.Snapshots {
			visited = append(visited, candidate.ID)
		}
		if page.Next == "" {
			break
		}
		if page.Next == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.Next
	}
	if len(visited) != len(candidates) {
		t.Fatal("candidate omitted", len(visited))
	}
	for index := range visited {
		if visited[index] != candidates[index].ID {
			t.Fatal("candidate repeated or reordered", index)
		}
	}
	for _, cursor := range []string{"../path", strings.Repeat("f", 64)} {
		if _, err := pageSnapshotCandidates(candidates, cursor); err == nil {
			t.Fatal("invalid or stale cursor accepted")
		}
	}
	page, err := pageSnapshotCandidates(nil, "")
	if err != nil || page.Snapshots == nil || len(page.Snapshots) != 0 || page.Next != "" {
		t.Fatal("empty page invalid", page, err)
	}
}
