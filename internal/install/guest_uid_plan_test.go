package install

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"strings"
	"testing"
)

func TestGuestUIDProvisioningPlanRefusesConflictWithoutShrinking(t *testing.T) {
	owner := strings.Repeat("a", 32)
	pool := supervisor.GuestUIDPool{First: 200000, Last: 200002}
	plan, err := guestUIDProvisioningPlan(context.Background(), owner, pool, []uint32{998, 997, 996})
	if err != nil || plan.First != pool.First || plan.Last != pool.Last || len(plan.Pending) == 0 {
		t.Fatal(plan, err)
	}
	for _, uid := range []uint32{pool.First, pool.First + 1, pool.Last} {
		pool.Blocked = map[uint32]bool{uid: true}
		denied, err := guestUIDProvisioningPlan(context.Background(), owner, pool, []uint32{998, 997})
		if err == nil || denied.OwnerID != "" || denied.First != 0 {
			t.Fatal("conflicting range produced usable plan", denied, err)
		}
	}
	pool.Blocked = nil
	for _, identities := range [][]uint32{{998}, {998, 998}, {0, 998}, {998, 200001}} {
		if _, err := guestUIDProvisioningPlan(context.Background(), owner, pool, identities); err == nil {
			t.Fatal("invalid service separation", identities)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanGuestUIDProvisioning(ctx, owner, pool, []uint32{998, 997}); err == nil {
		t.Fatal("cancelled host observation admitted")
	}
}
