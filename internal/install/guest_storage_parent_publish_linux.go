//go:build linux

package install

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Caller holds e.mu and retains image/parent/transition provenance, pathname and
// mount identity, account exclusion and blocked runtime activation. Its guard
// must admit only this recorded intermediate transition, never general drift.
// Existing snapshot-only recovery guards are insufficient for this publisher.
func (e *Engine) publishGuestStorageImageParentLocked(ctx context.Context, intent guestStorageImageParentIntent, transition guestStorageParentJournalIntent, parent *os.File, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if parent == nil || guard == nil || os.Geteuid() != 0 || intent.Version != 1 || transition.Version != 1 || intent.SourceGID == 0 || intent.SourceGID > math.MaxInt32 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, intent.Plan); err != nil {
		return err
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if digest(encoded) != transition.ParentIntentSHA256 {
		return ErrConflict
	}
	desired, err := planGuestStorageImageParentJournal(ctx, transition.Original, intent.SourceGID, intent.Plan.GuestGID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(desired, transition.Desired) {
		return ErrConflict
	}
	verify := func() (journal, unix.Stat_t, error) {
		empty := journal{}
		var st unix.Stat_t
		if err := guard(ctx); err != nil {
			return empty, st, err
		}
		current, err := e.load()
		if err != nil {
			return empty, st, err
		}
		if !reflect.DeepEqual(current, transition.Original) && !reflect.DeepEqual(current, transition.Desired) {
			return empty, st, ErrConflict
		}
		if unix.Fstat(int(parent.Fd()), &st) != nil || uint64(st.Dev) != intent.Device || st.Ino != intent.Inode || st.Mode != unix.S_IFDIR|0710 || st.Uid != 0 || (st.Gid != intent.SourceGID && st.Gid != intent.Plan.GuestGID) || (reflect.DeepEqual(current, transition.Desired) && st.Gid != intent.Plan.GuestGID) {
			return empty, st, ErrConflict
		}
		for _, item := range current.Items {
			if item.Path == "var/lib/homenode/images" {
				continue
			}
			if err := e.matches(item); err != nil {
				return empty, st, ErrConflict
			}
		}
		if err := guard(ctx); err != nil {
			return empty, st, err
		}
		final, err := e.load()
		var after unix.Stat_t
		if err != nil || !reflect.DeepEqual(final, current) || unix.Fstat(int(parent.Fd()), &after) != nil || after.Dev != st.Dev || after.Ino != st.Ino || after.Mode != st.Mode || after.Uid != st.Uid || after.Gid != st.Gid {
			return empty, st, ErrConflict
		}
		return current, st, ctx.Err()
	}
	current, st, err := verify()
	if err != nil {
		return err
	}
	if st.Gid != intent.Plan.GuestGID {
		if err := unix.Fchown(int(parent.Fd()), 0, int(intent.Plan.GuestGID)); err != nil {
			return err
		}
	}
	if err := parent.Sync(); err != nil {
		return err
	}
	if e.checkpoint != nil {
		if err := e.checkpoint("guest-storage-parent-ownership-migrated", "var/lib/homenode/images"); err != nil {
			return err
		}
	}
	current, _, err = verify()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, transition.Desired) {
		if err := e.save(transition.Desired); err != nil {
			return err
		}
	}
	if e.checkpoint != nil {
		if err := e.checkpoint("guest-storage-parent-journal-published", "install.json"); err != nil {
			return err
		}
	}
	current, st, err = verify()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, transition.Desired) || st.Gid != intent.Plan.GuestGID {
		return ErrConflict
	}
	return ctx.Err()
}
