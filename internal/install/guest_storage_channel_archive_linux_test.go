//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestRootGuestStorageChannelArchiveRequiresDifferentBootAndAbsentChildren(t *testing.T) {
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
	host, journalDir := roots(t)
	runtime := filepath.Join(host, "run", "homenode")
	if err := os.MkdirAll(runtime, 0755); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, journalDir)
	defer engine.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	stage, err := engine.stageGuestStorageChannelParent(ctx, plan, 1002, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.archivePreviousBootGuestStorageChannelStage(ctx, plan, 1002, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("current boot receipt archived", err)
	}
	stage.BootID = "12345678-1234-1234-1234-123456789abc"
	bootID, err := observeGuestStorageBootID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stage.BootID == bootID {
		stage.BootID = "22345678-1234-1234-1234-123456789abc"
	}
	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(journalDir, "guest-storage-channel-stage.json")
	if err := os.WriteFile(receipt, data, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.archivePreviousBootGuestStorageChannelStage(ctx, plan, 1002, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("occupied candidate permitted archival", err)
	}
	// Simulate a reboot's vacant runtime namespace; the operation itself must
	// never remove this candidate. This is not a physical reboot qualification.
	if err := os.Remove(filepath.Join(runtime, ".homenode-guests.stage")); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("archival acknowledgement interrupted")
	engine.checkpoint = func(name, path string) error {
		if name == "guest-storage-channel-prior-boot-archived" {
			return interrupted
		}
		return nil
	}
	if err := engine.archivePreviousBootGuestStorageChannelStage(ctx, plan, 1002, guard); !errors.Is(err, interrupted) {
		t.Fatal("archival interruption not observed", err)
	}
	engine.checkpoint = nil
	archive := filepath.Join(journalDir, "guest-storage-channel-stage."+stage.BootID+".json")
	after, err := os.Lstat(archive)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("archive replaced old receipt inode", err)
	}
	current, err := os.ReadFile(archive)
	if err != nil || string(current) != string(data) {
		t.Fatal("archive changed receipt evidence", err)
	}
	if _, err := os.Lstat(receipt); !os.IsNotExist(err) {
		t.Fatal("original receipt name retained", err)
	}
	// Resume the next transaction phase using a current-boot receipt. The
	// archived evidence must remain at its original inode throughout publication.
	fresh, err := engine.stageGuestStorageChannelParent(ctx, plan, 1002, guard)
	if err != nil || fresh.BootID != bootID {
		t.Fatal("current boot staging retry refused", fresh, err)
	}
	if err := engine.publishGuestStorageChannelParent(ctx, plan, 1002, fresh, guard); err != nil {
		t.Fatal("current boot publication retry refused", err)
	}
	retained, err := os.Lstat(archive)
	if err != nil || !os.SameFile(before, retained) {
		t.Fatal("reprovisioning replaced archived evidence", err)
	}
	current, err = os.ReadFile(archive)
	if err != nil || string(current) != string(data) {
		t.Fatal("reprovisioning changed archived evidence", err)
	}
}
