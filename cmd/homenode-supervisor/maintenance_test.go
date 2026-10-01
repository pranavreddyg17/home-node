package main

import (
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"testing"
)

func TestMaintenancePeerStartupPolicy(t *testing.T) {
	policy := supervisor.Policy{ControllerUID: 1001, TransferUID: 1002}
	for _, pair := range [][2]int{{-1, -1}, {1003, 2003}} {
		if err := validateMaintenancePeer(pair[0], pair[1], 2001, 2002, policy); err != nil {
			t.Fatal(pair, err)
		}
	}
	for _, pair := range [][2]int{{-1, 2003}, {1003, -1}, {0, 2003}, {1003, 0}, {1001, 2003}, {1002, 2003}, {1003, 2001}, {1003, 2002}, {-2, -2}} {
		if err := validateMaintenancePeer(pair[0], pair[1], 2001, 2002, policy); err == nil {
			t.Fatal("overlapping or incomplete maintenance identity accepted", pair)
		}
	}
}
