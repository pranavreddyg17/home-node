package install

import (
	"bytes"
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestStorageImagesIntentPreservesProvenance(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	image := guestStorageImageIdentity{OwnerID: identity.OwnerID, SHA256: strings.Repeat("b", 64), Bytes: 4096, SourceGID: 993, GuestGID: 994, Device: 1, Inode: 2}
	if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{image}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal, "guest-storage-images-intent.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{image}); err != nil {
		t.Fatal("exact retry refused", err)
	}
	changed := image
	changed.Inode++
	if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{changed}); !errors.Is(err, ErrConflict) {
		t.Fatal("replacement inode admitted", err)
	}
	changed = image
	changed.GuestGID++
	if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{changed}); !errors.Is(err, ErrPlan) {
		t.Fatal("unbound group admitted", err)
	}
	if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{image, image}); !errors.Is(err, ErrPlan) {
		t.Fatal("duplicate provenance admitted", err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("provenance inode changed", err)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, current) {
		t.Fatal("provenance bytes changed", err)
	}
	t.Run("interrupted creation", func(t *testing.T) {
		host, journal := roots(t)
		e := openEngine(t, host, journal)
		defer e.Close()
		interrupted := errors.New("interrupted image provenance")
		e.checkpoint = func(phase, path string) error {
			if phase == "guest-storage-images-intent-created" {
				return interrupted
			}
			return nil
		}
		if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{image}); !errors.Is(err, interrupted) {
			t.Fatal(err)
		}
		e.checkpoint = nil
		if err := e.commitGuestStorageImagesIntent(ctx, plan, []guestStorageImageIdentity{image}); !errors.Is(err, ErrConflict) {
			t.Fatal("partial provenance replaced", err)
		}
		data, err := os.ReadFile(filepath.Join(journal, "guest-storage-images-intent.json"))
		if err != nil || len(data) != 0 {
			t.Fatal("partial provenance repaired", err)
		}
	})
}
