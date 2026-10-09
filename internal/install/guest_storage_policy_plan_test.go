package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	env := []byte("TAILNET_IP=100.100.1.2\nHTTPS_PORT=8787\nHTTPS_ORIGIN=https://home.example.ts.net:8787\nPOLICY_GENERATION=1\nCONTROLLER_UID=1001\nRUNTIME_GID=1003\nTRANSFER_GID=1002\n")
	items := []record{{Path: "etc/homenode/runtime-policy.json", UID: 0, GID: 0, Mode: 0600, SHA256: digest(source), State: "pending"}, {Path: "var/lib/homenode/images", Directory: true, Mode: 0710, GID: 994, State: "pending"}}
	items = append(items, record{Path: "etc/homenode/services.env", Mode: 0644, SHA256: digest(env), State: "pending"})
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
	configuration, nextPolicy, nextEnv, err := planGuestStorageConfiguration(ctx, installed, source, env, plan)
	if err != nil || string(nextEnv) != strings.Replace(string(env), "POLICY_GENERATION=1\n", "POLICY_GENERATION=2\n", 1) || configuration.Items[0].SHA256 != digest(nextPolicy) || configuration.Items[2].SHA256 != digest(nextEnv) || !reflect.DeepEqual(configuration.Items[1], before[1]) {
		t.Fatal("configuration generations diverged", err)
	}
	for _, invalidEnv := range [][]byte{append(append([]byte(nil), env...), '\n'), []byte(strings.Replace(string(env), "POLICY_GENERATION=1", "POLICY_GENERATION=3", 1)), []byte(strings.Replace(string(env), "RUNTIME_GID=1003", "RUNTIME_GID=994", 1))} {
		if _, p, e, err := planGuestStorageConfiguration(ctx, installed, source, invalidEnv, plan); !errors.Is(err, ErrConflict) || p != nil || e != nil {
			t.Fatal("unbound environment returned transition", err)
		}
	}
	if _, _, err := planGuestStorageRuntimePolicy(ctx, installed, append(source, ' '), plan); !errors.Is(err, ErrConflict) {
		t.Fatal("unbound source admitted", err)
	}
	t.Run("immutable configuration intent", func(t *testing.T) {
		host, journalDir := roots(t)
		e := openEngine(t, host, journalDir)
		defer e.Close()
		guard := func(ctx context.Context) error { return ctx.Err() }
		if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, plan, guard); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(journalDir, "guest-storage-configuration-intent.json")
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, plan, guard); err != nil {
			t.Fatal("exact retry refused", err)
		}
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("retry replaced intent", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var intent guestStorageConfigurationIntent
		if json.Unmarshal(data, &intent) != nil || !reflect.DeepEqual(intent.Original, installed) || !reflect.DeepEqual(intent.Desired, configuration) || string(intent.Policy) != string(nextPolicy) || string(intent.Environment) != string(nextEnv) {
			t.Fatal("configuration transition lost authority")
		}
		changed := plan
		changed.GuestGID++
		if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, changed, guard); !errors.Is(err, ErrConflict) {
			t.Fatal("conflicting proposal replaced intent", err)
		}
		current, err := os.ReadFile(path)
		if err != nil || string(current) != string(data) {
			t.Fatal("conflict changed evidence", err)
		}
	})
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, data, err := planGuestStorageRuntimePolicy(cancelled, installed, source, plan); !errors.Is(err, context.Canceled) || data != nil {
		t.Fatal("cancelled policy returned", err)
	}
}
