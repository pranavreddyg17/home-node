package main

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestReservedRuntimePolicyRefusesPartialPeerAuthority(t *testing.T) {
	policy := supervisor.Policy{Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 << 30, ControllerUID: 1001, TransferUID: 1002,
		GuestIdentity: &supervisor.ReservedGuestPolicy{Version: 1, FirstUID: 200000, LastUID: 200002, GuestGID: 994}}
	for _, peers := range [][4]int{{0, 995, -1, -1}, {995, 0, -1, -1}, {995, 996, 200000, 997}, {995, 996, -1, 997}, {995, 996, 1003, -1}} {
		pool, err := loadReservedRuntimePolicy(context.Background(), policy, "", "", "", peers[0], peers[1], peers[2], peers[3])
		if !errors.Is(err, supervisor.ErrPolicy) || pool.First != 0 || pool.Last != 0 || pool.Blocked != nil {
			t.Fatal("partial peer policy admitted", peers, pool, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool, err := loadReservedRuntimePolicy(ctx, policy, "", "", "", 995, 996, -1, -1)
	if !errors.Is(err, context.Canceled) || pool.First != 0 || pool.Last != 0 {
		t.Fatal("cancelled policy admitted", pool, err)
	}
}
