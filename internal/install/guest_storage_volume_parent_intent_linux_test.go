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
	if err := e.withRecordedGuestStorageVolumeParent(ctx, intent, guard, func(_ *os.Root, _ *os.File, check func(context.Context) error) error { return check(ctx) }); err != nil {
		t.Fatal("recorded parent refused", err)
	}
	if err := e.withRecordedGuestStorageVolumeParent(ctx, intent, guard, func(_ *os.Root, _ *os.File, check func(context.Context) error) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0710); err != nil {
			return err
		}
		if err := os.Chown(path, 0, 993); err != nil {
			return err
		}
		if err := os.Chmod(path, 0710); err != nil {
			return err
		}
		return check(ctx)
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("replacement volume parent admitted", err)
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

func TestRootEmptyGuestStorageVolumeParentPublicationRecoversOwnershipInterruption(t *testing.T) {
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

	if err := e.save(installed); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(path, "foreign-volume")
	if err := os.WriteFile(occupied, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.publishEmptyGuestStorageVolumeParentLocked(ctx, plan, 993, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("populated directory admitted", err)
	}
	data, err := os.ReadFile(occupied)
	if err != nil || string(data) != "preserve" {
		t.Fatal("refusal altered foreign volume", err)
	}
	if err := os.Remove(occupied); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("ownership acknowledgement interrupted")
	e.checkpoint = func(stage, name string) error {
		if stage == "guest-storage-volume-parent-ownership-migrated" {
			return interrupted
		}
		return nil
	}
	if err := e.publishEmptyGuestStorageVolumeParentLocked(ctx, plan, 993, guard); !errors.Is(err, interrupted) {
		t.Fatal("ownership interruption not observed", err)
	}
	observed, err := e.load()
	if err != nil || observed.Digest != installed.Digest {
		t.Fatal("journal committed before ownership acknowledgement", err)
	}
	e.checkpoint = nil
	if err := e.publishEmptyGuestStorageVolumeParentLocked(ctx, plan, 993, guard); err != nil {
		t.Fatal("recorded ownership retry refused", err)
	}
	desired, err := planGuestStorageVolumeParentJournal(ctx, installed, 993, 994)
	if err != nil {
		t.Fatal(err)
	}
	observed, err = e.load()
	if err != nil || observed.Digest != desired.Digest {
		t.Fatal("destination journal not committed", err)
	}
	before, err := os.Stat(filepath.Join(journalDir, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.publishEmptyGuestStorageVolumeParentLocked(ctx, plan, 993, guard); err != nil {
		t.Fatal("completed retry refused", err)
	}
	after, err := os.Stat(filepath.Join(journalDir, "install.json"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("completed retry replaced journal", err)
	}
}
