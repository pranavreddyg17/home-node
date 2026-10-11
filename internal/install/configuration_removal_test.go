package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationRemovalPlanRetainsOccupiedDataAndExistingPaths(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	plan := fixturePlan()
	uid, gid := os.Geteuid(), os.Getegid()
	plan.Items = append(plan.Items, Item{Path: "var/lib/homenode/catalog", Directory: true, Mode: 0755, UID: uid, GID: gid}, Item{Path: "var/lib/homenode/catalog/catalog.json", Mode: 0640, UID: uid, GID: gid, Data: []byte("retained catalog")}, Item{Path: "var/lib/homenode-backup", Directory: true, Mode: 0755, UID: uid, GID: gid})
	// An already existing directory is retained rather than adopted for removal.
	if err := os.Mkdir(filepath.Join(host, "etc/homenode"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(host, "var/lib/homenode/control/management.db")
	backupPath := filepath.Join(host, "var/lib/homenode-backup/owner-data")
	for _, p := range []string{dataPath, backupPath} {
		if err := os.WriteFile(p, []byte("preserve owner data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e.mu.Lock()
	removal, err := e.planConfigurationRemovalLocked(context.Background())
	e.mu.Unlock()
	if err != nil || len(removal.Files) != 1 || removal.Files[0].Path != "etc/homenode/runtime-policy.json" || len(removal.RetainedPaths) != 6 {
		t.Fatal(removal, err)
	}
	for _, p := range []string{dataPath, backupPath} {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != "preserve owner data" {
			t.Fatal("planning changed data", err)
		}
	}
	if _, err := os.Stat(filepath.Join(host, "etc/homenode/runtime-policy.json")); err != nil {
		t.Fatal("planning removed configuration", err)
	}
}

func TestConfigurationRemovalPlanRefusesDriftAndCancellation(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	ctx := context.Background()
	if _, err := e.planConfigurationRemovalLocked(ctx); err == nil {
		t.Fatal("missing provenance admitted")
	}
	if err := e.Apply(ctx, fixturePlan()); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.planConfigurationRemovalLocked(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	p := filepath.Join(host, "etc/homenode/runtime-policy.json")
	if err := os.WriteFile(p, []byte("foreign configuration"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := e.planConfigurationRemovalLocked(ctx); err == nil {
		t.Fatal("changed configuration admitted")
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "foreign configuration" {
		t.Fatal("planning changed foreign file", err)
	}
}
