//go:build linux

package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

func TestSnapshotPreviewPacketRequiresOwnRequestSelectionAndLiveContext(t *testing.T) {
	request := strings.Repeat("a", 32)
	manifest, policy, _ := manifestFixture()
	snapshot := strings.Repeat("b", 64)
	preview, err := previewSnapshotManifest(snapshot, manifest, policy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeSnapshotPreviewResponse(request, preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "foreign", "foreign-snapshot", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			client, server := dispatchPair(t)
			if err := disktransport.SendPacket(context.Background(), server, raw); err != nil {
				t.Fatal(err)
			}
			expected := request
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "foreign" {
				expected = strings.Repeat("b", 32)
			}
			if scenario == "cancelled" {
				cancel()
			}
			selected := snapshot
			if scenario == "foreign-snapshot" {
				selected = strings.Repeat("c", 64)
			}
			page, err := receiveSnapshotPreviewResponse(ctx, client, expected, selected)
			if scenario == "valid" {
				if err != nil || page.SnapshotID != snapshot || page.Files == nil {
					t.Fatal(page, err)
				}
			} else if err == nil || page.Files != nil {
				t.Fatal("unqualified packet returned candidates", page, err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
		})
	}
}
