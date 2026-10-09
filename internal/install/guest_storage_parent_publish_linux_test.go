//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"golang.org/x/sys/unix"
)

func TestRootGuestStorageParentPublicationRecoversOwnershipInterruption(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	e, c, _, journalDirectory, _, _ := imagePlacementFixtureWithMaintenance(t, &MaintenanceAccount{UID: 803, GID: 803})
	defer e.Close()
	ctx := context.Background()
	if err := e.blockRecoveryActivation(ctx); err != nil {
		t.Fatal(err)
	}
	installed, err := e.load()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := e.host.Open("var/lib/homenode/images")
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	var st unix.Stat_t
	if err := unix.Fstat(int(parent.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	intent := guestStorageImageParentIntent{Version: 1, Plan: plan, ImagesIntentSHA256: strings.Repeat("b", 64), SourceGID: uint32(c.Accounts.QEMUGID), Device: uint64(st.Dev), Inode: st.Ino}
	encoded, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestStorageParentJournalIntent(ctx, installed, intent.SourceGID, plan.GuestGID, digest(encoded)); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("interrupted before parent journal publication")
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-parent-ownership-migrated" {
			return failure
		}
		return nil
	}
	publish := func() error {
		e.mu.Lock()
		defer e.mu.Unlock()
		current, err := e.load()
		if err != nil {
			return err
		}
		return e.withGuestStorageParentJournalIntent(ctx, current, intent.SourceGID, plan.GuestGID, digest(encoded), func(transition guestStorageParentJournalIntent, check func() error) error {
			return e.withRecordedGuestStorageImageParent(ctx, intent, func(ctx context.Context) error { return check() }, func(root *os.Root, pinned *os.File, checkPath func(context.Context) error) error {
				admit := func(ctx context.Context, current journal) error {
					if err := check(); err != nil {
						return err
					}
					if err := e.admitGuestStorageParentInstallation(ctx, current, intent, transition, pinned); err != nil {
						return err
					}
					return check()
				}
				return e.withRecoveryInstallationExclusionGuardedLocked(ctx, func(ctx context.Context) error { return ctx.Err() }, e.observeRecoveryDestinationVacancy, admit, func(ctx context.Context, guard func(context.Context) error) error {
					return e.withGuestStorageAccountExclusionLocked(ctx, guard, func(ctx context.Context, guard func(context.Context) error) error {
						combined := func(ctx context.Context) error {
							if err := checkPath(ctx); err != nil {
								return err
							}
							if err := guard(ctx); err != nil {
								return err
							}
							return checkPath(ctx)
						}
						return e.publishGuestStorageImageParentLocked(ctx, intent, transition, pinned, combined)
					})
				})
			})
		})
	}
	if err := publish(); !errors.Is(err, failure) {
		t.Fatal("interruption lost", err)
	}
	current, err := e.load()
	if err != nil || !reflect.DeepEqual(current, installed) {
		t.Fatal("journal published before ownership acknowledgement", err)
	}
	if err := unix.Fstat(int(parent.Fd()), &st); err != nil || st.Gid != plan.GuestGID {
		t.Fatal("intermediate ownership not retained", err)
	}
	e.checkpoint = func(phase, path string) error {
		if phase == "guest-storage-parent-ownership-migrated" {
			return parent.Chown(0, int(intent.SourceGID))
		}
		return nil
	}
	if err := publish(); !errors.Is(err, ErrConflict) {
		t.Fatal("journal publication admitted reverted source ownership", err)
	}
	current, err = e.load()
	if err != nil || !reflect.DeepEqual(current, installed) {
		t.Fatal("journal published with source ownership", err)
	}
	e.checkpoint = nil
	if err := publish(); err != nil {
		t.Fatal("recorded intermediate retry refused", err)
	}
	current, err = e.load()
	expected, planErr := planGuestStorageImageParentJournal(ctx, installed, intent.SourceGID, plan.GuestGID)
	if err != nil || planErr != nil || !reflect.DeepEqual(current, expected) {
		t.Fatal("destination journal mismatch", err, planErr)
	}
	path := filepath.Join(journalDirectory, "install.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publish(); err != nil {
		t.Fatal("completed retry refused", err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("completed retry rewrote journal", err)
	}
	if err := parent.Chown(0, int(intent.SourceGID)); err != nil {
		t.Fatal(err)
	}
	if err := publish(); !errors.Is(err, ErrConflict) {
		t.Fatal("destination journal adopted reverted parent", err)
	}
	if err := unix.Fstat(int(parent.Fd()), &st); err != nil || st.Gid != intent.SourceGID {
		t.Fatal("refusal repaired reverted parent", err)
	}
	if err := e.requireRecoveryActivationBlock(ctx); err != nil {
		t.Fatal("publication released activation marker", err)
	}
}
