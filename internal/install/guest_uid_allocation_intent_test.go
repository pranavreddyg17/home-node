package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestUIDAllocationIntentRetainsLargeConfigurationAndRejectsDrift(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx := context.Background()
	selection := guestUIDAllocatorRanges{NormalFirst: 1000, NormalLast: 60000, SystemFirst: 100, SystemLast: 999, SubordinateFirst: 100000, SubordinateLast: 600100000}
	original := strings.Repeat("# retained allocator documentation\n", 400) + "UID_MIN 1000\nUID_MAX 60000\n"
	pool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
	proposal, err := planGuestUIDAllocatorConfiguration(ctx, []byte(original), pool, selection)
	if err != nil {
		t.Fatal(err)
	}
	intent := guestUIDAllocationIntent{Version: 1, OwnerID: strings.Repeat("a", 32), First: pool.First, Last: pool.Last, Selection: selection, Original: original, Proposal: proposal}
	if err := e.commitGuestUIDAllocationIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal, "guest-uid-allocation-intent.json")
	before, err := os.Lstat(path)
	if err != nil || before.Size() <= 8192 {
		t.Fatal("large allocator intent was not durably represented", err)
	}
	if err := e.commitGuestUIDAllocationIntent(ctx, intent); err != nil {
		t.Fatal("exact allocator retry", err)
	}
	loaded, err := e.loadGuestUIDAllocationIntent(ctx, intent.OwnerID)
	if err != nil || loaded != intent {
		t.Fatal("allocator restart intent changed approved selection", err)
	}
	loaded, err = e.loadGuestUIDAllocationIntent(ctx, strings.Repeat("b", 32))
	if !errors.Is(err, ErrConflict) || loaded != (guestUIDAllocationIntent{}) {
		t.Fatal("allocator restart adopted foreign owner", err)
	}
	changed := intent
	changed.OwnerID = strings.Repeat("b", 32)
	if err := e.commitGuestUIDAllocationIntent(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("allocator owner conflict adopted", err)
	}
	changed = intent
	changed.Proposal.Contents += "UID_MIN 1000\n"
	if err := e.commitGuestUIDAllocationIntent(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("unplanned allocator bytes committed", err)
	}
	called := false
	if err := e.withGuestUIDAllocationIntent(ctx, intent, func(_ context.Context, check func() error) error { called = true; return check() }); err != nil || !called {
		t.Fatal("large allocator authority could not be retained", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitImmutableGuestIntent(ctx, "guest-identity-nss-intent.json", "guest-identity-nss-intent", data); !errors.Is(err, ErrPlan) {
		t.Fatal("allocator sizing widened unrelated intent admission", err)
	}
	if err := e.withGuestIdentityRecordGuarded(ctx, "guest-identity-nss-intent.json", data, func(context.Context, func() error) error { return nil }); !errors.Is(err, ErrPlan) {
		t.Fatal("allocator sizing widened unrelated retained record scope", err)
	}
	err = e.withGuestUIDAllocationIntent(ctx, intent, func(_ context.Context, check func() error) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
		return check()
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("identical-byte allocator record replacement retained authority", err)
	}
}
