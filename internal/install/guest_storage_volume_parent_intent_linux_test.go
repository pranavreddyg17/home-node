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

func TestRootGuestStorageVolumeParentIntentRetainsOriginalInode(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture")
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
	items := []record{{Path: "var/lib/homenode/volumes", Directory: true, GID: 993, Mode: 0710, State: "pending"}}
	encoded, _ := json.Marshal(items)
	installed := journal{Version: 1, ID: identity.OwnerID, Phase: "installed", Digest: digest(encoded), Items: items}
	installed.Items[0].State = "created"
	host, journalDir := roots(t)
	path := filepath.Join(host, "var/lib/homenode/volumes")
	if err := os.MkdirAll(path, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 993); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0710); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	e := openEngine(t, host, journalDir)
	defer e.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := e.commitGuestStorageVolumeParentIntent(ctx, installed, plan, 993, parent, guard); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(journalDir, "guest-storage-volume-parent-intent.json")
	before, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageVolumeParentIntent(ctx, installed, plan, 993, parent, guard); err != nil {
		t.Fatal("exact retry refused", err)
	}
	after, err := os.Stat(recordPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("retry replaced provenance", err)
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var intent guestStorageVolumeParentIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		t.Fatal(err)
	}
	for _, current := range []journal{installed, intent.Desired} {
		if err := e.withGuestStorageVolumeParentIntent(ctx, current, plan, 993, func(guestStorageVolumeParentIntent, func() error) error { return nil }); err != nil {
			t.Fatal("recorded journal state refused", err)
		}
	}
	changed := plan
	changed.GuestGID++
	if err := e.commitGuestStorageVolumeParentIntent(ctx, installed, changed, 993, parent, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("conflicting destination accepted", err)
	}
	info, err := parent.Stat()
	if err != nil || info.Mode().Perm() != 0710 {
		t.Fatal("intent changed parent", err)
	}
	if err := e.withGuestStorageVolumeParentIntent(ctx, installed, plan, 993, func(_ guestStorageVolumeParentIntent, check func() error) error {
		if err := os.Rename(recordPath, recordPath+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(recordPath, data, 0600); err != nil {
			return err
		}
		return check()
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("identical intent replacement admitted", err)
	}
	for _, path := range []string{recordPath, recordPath + ".original"} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != string(data) {
			t.Fatal("conflict changed evidence", err)
		}
	}

}
