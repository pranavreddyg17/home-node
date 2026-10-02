//go:build linux

package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

func TestSnapshotPagePacketRequiresOwnRequestAndLiveContext(t *testing.T) {
	request := strings.Repeat("a", 32)
	raw, err := EncodeSnapshotPageResponse(request, SnapshotPage{Snapshots: []SnapshotReference{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "foreign", "cancelled"} {
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
			page, err := receiveSnapshotPageResponse(ctx, client, expected)
			if scenario == "valid" {
				if err != nil || page.Snapshots == nil || len(page.Snapshots) != 0 {
					t.Fatal(page, err)
				}
			} else if err == nil || page.Snapshots != nil {
				t.Fatal("unqualified packet returned candidates", page, err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
		})
	}
}
