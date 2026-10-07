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

func TestGuestUIDIntentExactRetryAndPolicyDrift(t *testing.T) {
	host, jr := roots(t)
	engine := openEngine(t, host, jr)
	defer engine.Close()
	ctx := context.Background()
	plan, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200001}, []uint32{998, 997})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.commitGuestUIDIntent(ctx, plan); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(jr, "guest-uid-intent.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.commitGuestUIDIntent(ctx, plan); err != nil {
		t.Fatal("exact retry refused", err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0600 {
		t.Fatal("retry replaced intent", err)
	}
	for _, change := range []string{"owner", "range", "service", "pending"} {
		changed := plan
		changed.ServiceUIDs = append([]uint32(nil), plan.ServiceUIDs...)
		changed.Pending = append([]string(nil), plan.Pending...)
		switch change {
		case "owner":
			changed.OwnerID = strings.Repeat("b", 32)
		case "range":
			changed.Last++
		case "service":
			changed.ServiceUIDs[0] = 996
		case "pending":
			changed.Pending = nil
		}
		if err := engine.commitGuestUIDIntent(ctx, changed); err == nil {
			t.Fatal("changed intent adopted", change)
		}
		contents, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(contents, original) {
			t.Fatal("changed intent overwritten", change, err)
		}
	}
}

func TestGuestUIDIntentPreservesAmbiguousExistingRecord(t *testing.T) {
	for _, kind := range []string{"truncated", "alias", "symlink", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			host, jr := roots(t)
			engine := openEngine(t, host, jr)
			defer engine.Close()
			ctx := context.Background()
			plan, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200001}, []uint32{998, 997})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(jr, "guest-uid-intent.json")
			preserved := path
			if kind == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if err := engine.commitGuestUIDIntent(cancelled, plan); err == nil {
					t.Fatal("cancelled intent committed")
				}
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("cancelled intent created path", err)
				}
				return
			}
			if err := engine.commitGuestUIDIntent(ctx, plan); err != nil {
				t.Fatal(err)
			}
			if kind == "truncated" {
				if err := os.WriteFile(path, []byte("{\"version\":"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "alias" {
				preserved = filepath.Join(jr, "alias")
				if err := os.Link(path, preserved); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink" {
				preserved = filepath.Join(jr, "target")
				if err := os.Rename(path, preserved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("target", path); err != nil {
					t.Fatal(err)
				}
			}
			original, err := os.ReadFile(preserved)
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.commitGuestUIDIntent(ctx, plan); err == nil {
				t.Fatal("ambiguous intent adopted", kind)
			}
			contents, err := os.ReadFile(preserved)
			if err != nil || !bytes.Equal(original, contents) {
				t.Fatal("uncertain intent changed", err)
			}
		})
	}
}
