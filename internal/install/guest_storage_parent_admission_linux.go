//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Retained provenance and pathname/mount checks remain the caller's duty.
// This permits precisely one parent transition and checks every other record.
func (e *Engine) admitGuestStorageParentInstallation(ctx context.Context, current journal, intent guestStorageImageParentIntent, transition guestStorageParentJournalIntent, parent *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if parent == nil || intent.Version != 1 || transition.Version != 1 {
		return ErrPlan
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if digest(data) != transition.ParentIntentSHA256 {
		return ErrConflict
	}
	desired, err := planGuestStorageImageParentJournal(ctx, transition.Original, intent.SourceGID, intent.Plan.GuestGID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(desired, transition.Desired) || (!reflect.DeepEqual(current, transition.Original) && !reflect.DeepEqual(current, transition.Desired)) {
		return ErrConflict
	}
	var st unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &st) != nil || uint64(st.Dev) != intent.Device || st.Ino != intent.Inode || st.Mode != unix.S_IFDIR|0710 || st.Uid != 0 || (st.Gid != intent.SourceGID && st.Gid != intent.Plan.GuestGID) || (reflect.DeepEqual(current, transition.Desired) && st.Gid != intent.Plan.GuestGID) {
		return ErrConflict
	}
	for _, item := range current.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Path == "var/lib/homenode/images" {
			continue
		}
		if err := e.matches(item); err != nil {
			return ErrConflict
		}
	}
	var final unix.Stat_t
	loaded, err := e.load()
	if err != nil || !reflect.DeepEqual(loaded, current) || unix.Fstat(int(parent.Fd()), &final) != nil || final.Dev != st.Dev || final.Ino != st.Ino || final.Mode != st.Mode || final.Uid != st.Uid || final.Gid != st.Gid {
		return ErrConflict
	}
	return ctx.Err()
}
