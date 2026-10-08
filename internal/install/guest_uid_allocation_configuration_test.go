package install

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestUIDAllocatorProposalPreservesUnrelatedSettings(t *testing.T) {
	original := []byte("# dedicated host\nUID_MIN 1000\nUID_MAX 60000\n# SYS_UID_MIN 100\nPASS_MAX_DAYS 99999\nSUB_UID_MIN 100000\nSUB_UID_MAX 600100000")
	selection := guestUIDAllocatorRanges{NormalFirst: 1000, NormalLast: 60000, SystemFirst: 100, SystemLast: 999, SubordinateFirst: 100000, SubordinateLast: 600100000}
	pool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
	proposal, err := planGuestUIDAllocatorConfiguration(context.Background(), original, pool, selection)
	if err != nil || proposal.OriginalSHA256 != digest(original) || proposal.DesiredSHA256 != digest([]byte(proposal.Contents)) || !strings.Contains(proposal.Contents, "# SYS_UID_MIN 100\nPASS_MAX_DAYS 99999\n") || !strings.HasSuffix(proposal.Contents, "SYS_UID_MIN 100\nSYS_UID_MAX 999\n") {
		t.Fatal("explicit allocator proposal lost host bindings", err, proposal)
	}
	if err := supervisor.ValidateGuestUIDAutomaticAllocationConfiguration(context.Background(), pool, []byte(proposal.Contents)); err != nil {
		t.Fatal("proposed ranges do not exclude selected guest pool", err)
	}
	replay, err := planGuestUIDAllocatorConfiguration(context.Background(), []byte(proposal.Contents), pool, selection)
	if err != nil || replay.Contents != proposal.Contents || replay.OriginalSHA256 != replay.DesiredSHA256 {
		t.Fatal("applied allocator configuration did not produce a stable proposal", err)
	}
	for _, bad := range []string{string(original) + "\nUID_MIN 1000\n", "UID_MIN remote-value\n", "UID_MIN 4294967296\n", "UID_MIN 1000 extra\n", "# config\x00", "# config\r\n"} {
		result, err := planGuestUIDAllocatorConfiguration(context.Background(), []byte(bad), pool, selection)
		if err == nil || result != (guestUIDAllocationProposal{}) {
			t.Fatal("ambiguous allocator source supplied publication bytes", err)
		}
	}
	selection.SubordinateLast = pool.First
	if result, err := planGuestUIDAllocatorConfiguration(context.Background(), original, pool, selection); !errors.Is(err, supervisor.ErrPolicy) || result != (guestUIDAllocationProposal{}) {
		t.Fatal("overlapping explicit ranges supplied proposal", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := planGuestUIDAllocatorConfiguration(ctx, original, pool, selection); !errors.Is(err, context.Canceled) || result != (guestUIDAllocationProposal{}) {
		t.Fatal("cancelled allocator proposal supplied bytes", err)
	}
}
