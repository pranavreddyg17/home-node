//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootGuestStorageConfigurationSourceRefusesReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	for _, name := range []string{"runtime-policy.json", "services.env"} {
		mode := os.FileMode(0644)
		if name == "runtime-policy.json" {
			mode = 0600
		}
		path := filepath.Join(directory, name)
		original := "owned configuration\n"
		if err := os.WriteFile(path, []byte(original), mode); err != nil {
			t.Fatal(err)
		}
		if err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error { return check() }); err != nil {
			t.Fatal(err)
		}
		err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(original), mode); err != nil {
				return err
			}
			if err := check(); !errors.Is(err, ErrConflict) {
				t.Fatal("identical replacement admitted", err)
			}
			return nil
		})
		if !errors.Is(err, ErrConflict) {
			t.Fatal("replacement scope succeeded", err)
		}
		for _, p := range []string{path, path + ".original"} {
			data, err := os.ReadFile(p)
			if err != nil || string(data) != original {
				t.Fatal("replacement evidence altered", err)
			}
		}
	}
}

func TestRootGuestStorageConfigurationSourcesRetainBothFiles(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
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

	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(directory, "runtime-policy.json")
	envPath := filepath.Join(directory, "services.env")
	if err := os.WriteFile(policyPath, source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, env, 0644); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, plan, guard); err != nil {
		t.Fatal(err)
	}
	consume := func(_ context.Context, _ guestStorageConfigurationIntent, _ *os.File, _ *os.File, check func() error) error {
		return check()
	}
	if err := e.withGuestStorageConfigurationSources(ctx, installed, plan, guard, consume); err != nil {
		t.Fatal(err)
	}
	err = e.withGuestStorageConfigurationSources(ctx, installed, plan, guard, func(_ context.Context, _ guestStorageConfigurationIntent, _ *os.File, _ *os.File, check func() error) error {
		if err := os.Rename(policyPath, policyPath+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(policyPath, source, 0600); err != nil {
			return err
		}
		if err := check(); !errors.Is(err, ErrConflict) {
			t.Fatal("policy replacement hidden while environment retained", err)
		}
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("joint scope accepted replaced policy", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil || string(data) != string(env) {
		t.Fatal("environment changed on refusal", err)
	}
}
