package install

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"strings"
	"testing"
)

func TestGuestStoragePlanRefusesProtectedGroupsAndIdentityDrift(t *testing.T) {
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	protected := []int{1001, 1002, 1003, 1004}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 993, protected)
	if err != nil || plan.GuestGID != 993 || plan.ParentMode != 0710 || plan.ImageMode != 0440 || plan.VolumeMode != 0600 {
		t.Fatal("qualified layout refused", plan, err)
	}
	for _, gid := range []uint32{0, 1 << 31, 1001, 1002, 1003, 1004} {
		if plan, err := guestStorageProvisioningPlan(ctx, identity, gid, protected); err == nil || plan.GuestGID != 0 {
			t.Fatal("protected or invalid group admitted", gid, plan, err)
		}
	}
	for _, groups := range [][]int{nil, {1001, 1002}, {1001, 1002, 0}, {1001, 1002, -1}, {1001, 1002, 1 << 31}} {
		if _, err := guestStorageProvisioningPlan(ctx, identity, 993, groups); err == nil {
			t.Fatal("invalid protected groups admitted", groups)
		}
	}
	identity.Pending = []string{"activate now"}
	if _, err := guestStorageProvisioningPlan(ctx, identity, 993, protected); !errors.Is(err, ErrConflict) {
		t.Fatal("identity pending gates silently repaired", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := guestStorageProvisioningPlan(cancelled, identity, 993, protected); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
