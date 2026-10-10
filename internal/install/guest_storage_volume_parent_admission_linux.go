//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Read-only admission for runtime exclusion during an empty-parent transition.
// Caller retains the authenticated intent, pathname/mount and account guards.
func (e *Engine) admitEmptyGuestStorageVolumeParentInstallation(ctx context.Context, current journal, intent guestStorageVolumeParentIntent, root *os.Root, parent *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if intent.Version != 1 || root == nil || parent == nil {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, intent.Plan); err != nil {
		return err
	}
	desired, err := planGuestStorageVolumeParentJournal(ctx, intent.Original, intent.SourceGID, intent.Plan.GuestGID)
	if err != nil {
		return err
	}
	if intent.Original.ID != intent.Plan.Identity.OwnerID || !reflect.DeepEqual(desired, intent.Desired) || (!reflect.DeepEqual(current, intent.Original) && !reflect.DeepEqual(current, intent.Desired)) {
		return ErrConflict
	}
	var original unix.Stat_t
	opened, err := parent.Stat()
	rootInfo, rootErr := root.Stat(".")
	if err != nil || rootErr != nil || !os.SameFile(opened, rootInfo) || unix.Fstat(int(parent.Fd()), &original) != nil || uint64(original.Dev) != intent.Device || original.Ino != intent.Inode || original.Mode != unix.S_IFDIR|0710 || original.Uid != 0 || (original.Gid != intent.SourceGID && original.Gid != intent.Plan.GuestGID) || (reflect.DeepEqual(current, intent.Desired) && original.Gid != intent.Plan.GuestGID) {
		return ErrConflict
	}
	reader, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := reader.ReadDir(1)
	closeErr := reader.Close()
	if len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
		return ErrConflict
	}
	for _, item := range current.Items {
		if item.Path == "var/lib/homenode/volumes" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.matches(item); err != nil {
			return ErrConflict
		}
	}
	loaded, err := e.load()
	var after unix.Stat_t
	if err != nil || !reflect.DeepEqual(loaded, current) || unix.Fstat(int(parent.Fd()), &after) != nil || after.Dev != original.Dev || after.Ino != original.Ino || after.Mode != original.Mode || after.Uid != original.Uid || after.Gid != original.Gid {
		return ErrConflict
	}
	return ctx.Err()
}
