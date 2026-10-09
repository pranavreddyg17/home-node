package install

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestStoragePolicyPlanBindsInstalledBytesAndPreservesOtherRecords(t *testing.T) {
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	policy := supervisor.Policy{Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 << 30, ControllerUID: 1001, TransferUID: 1002}
	source, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, '\n')
	items := []record{{Path: "etc/homenode/runtime-policy.json", UID: 0, GID: 0, Mode: 0600, SHA256: digest(source), State: "pending"}, {Path: "var/lib/homenode/images", Directory: true, Mode: 0710, GID: 994, State: "pending"}}
	encoded, _ := json.Marshal(items)
	installed := journal{Version: 1, ID: strings.Repeat("a", 32), Phase: "installed", Digest: digest(encoded), Items: items}
	for i := range installed.Items {
		installed.Items[i].State = "created"
	}
	before := append([]record(nil), installed.Items...)
	desired, data, err := planGuestStorageRuntimePolicy(ctx, installed, source, plan)
	if err != nil {
		t.Fatal(err)
	}
	var next supervisor.Policy
	if json.Unmarshal(data, &next) != nil || next.Generation != 2 || next.GuestIdentity == nil || next.GuestIdentity.GuestGID != 994 {
		t.Fatal("incorrect policy transition", next)
	}
	if !reflect.DeepEqual(installed.Items, before) || !reflect.DeepEqual(desired.Items[1], before[1]) || desired.Items[0].SHA256 != digest(data) {
		t.Fatal("unrelated authority changed")
	}
	if _, _, err := planGuestStorageRuntimePolicy(ctx, installed, append(source, ' '), plan); !errors.Is(err, ErrConflict) {
		t.Fatal("unbound source admitted", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, data, err := planGuestStorageRuntimePolicy(cancelled, installed, source, plan); !errors.Is(err, context.Canceled) || data != nil {
		t.Fatal("cancelled policy returned", err)
	}
}
