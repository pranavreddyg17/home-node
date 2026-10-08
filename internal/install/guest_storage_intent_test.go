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

func TestGuestStorageIntentRetainsIdentityAcrossRetryAndConflict(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 993, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageIntent(ctx, plan); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(jr, "guest-storage-intent.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageIntent(ctx, plan); err != nil {
		t.Fatal("exact retry", err)
	}
	for _, changed := range []GuestStorageProvisioningPlan{
		{Identity: identity, GuestGID: 994, ParentMode: 0710, ImageMode: 0440, VolumeMode: 0600},
		{Identity: identity, GuestGID: 993, ParentMode: 0755, ImageMode: 0440, VolumeMode: 0600},
		{Identity: identity, GuestGID: 993, ParentMode: 0710, ImageMode: 0640, VolumeMode: 0600},
	} {
		if err := e.commitGuestStorageIntent(ctx, changed); err == nil {
			t.Fatal("changed storage identity admitted")
		}
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Mode().Perm() != 0600 {
		t.Fatal("intent identity changed", err)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("conflict rewrote intent", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := e.commitGuestStorageIntent(cancelled, plan); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

func TestGuestStorageIntentPreservesInterruptedCreation(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 993, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("fixture interruption")
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-intent-created" {
			return interrupted
		}
		return nil
	}
	if err := e.commitGuestStorageIntent(ctx, plan); !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	e.checkpoint = nil
	if err := e.commitGuestStorageIntent(ctx, plan); !errors.Is(err, ErrConflict) {
		t.Fatal("partial intent silently replaced", err)
	}
	data, err := os.ReadFile(filepath.Join(jr, "guest-storage-intent.json"))
	if err != nil || len(data) != 0 {
		t.Fatal("partial intent changed", err)
	}
}
