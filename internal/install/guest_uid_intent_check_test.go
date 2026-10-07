package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestUIDIntentLoadRequiresCanonicalPendingGates(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	ctx := context.Background()
	plan, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}, []uint32{998, 997})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.commitGuestUIDIntent(ctx, plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.loadGuestUIDIntent(ctx)
	if err != nil || loaded.OwnerID != plan.OwnerID || loaded.First != plan.First || len(loaded.Pending) != len(plan.Pending) {
		t.Fatal("canonical intent refused", loaded, err)
	}
	path := filepath.Join(jr, "guest-uid-intent.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range [][]byte{
		append(append([]byte(nil), original...), '\n'),
		bytes.Replace(original, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(original, []byte(`"version":1`), []byte(`"version":2`), 1),
		bytes.Replace(original, []byte(`"version":1`), []byte(`"version":1,"activationQualified":true`), 1),
		bytes.Replace(original, []byte(plan.Pending[0]), []byte("activation approved"), 1),
	} {
		if err := os.WriteFile(path, changed, 0600); err != nil {
			t.Fatal(err)
		}
		denied, err := engine.loadGuestUIDIntent(ctx)
		if err == nil || denied.OwnerID != "" || denied.ServiceUIDs != nil {
			t.Fatal("noncanonical or weakened intent admitted", denied, err)
		}
		preserved, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(preserved, changed) {
			t.Fatal("invalid intent repaired", err)
		}
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(jr, "alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.loadGuestUIDIntent(ctx); err == nil {
		t.Fatal("aliased intent admitted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := engine.loadGuestUIDIntent(cancelled); err == nil {
		t.Fatal("cancelled intent read admitted")
	}
}
