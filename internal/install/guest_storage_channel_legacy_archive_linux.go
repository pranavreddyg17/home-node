//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Caller retains qualified installed/runtime/account authority. Preserve the
// recorded legacy inode and receipt; do not change its group or contents.
func (e *Engine) archiveGuestStorageLegacyChannel(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	return e.withGuestStorageLegacyChannelIntent(ctx, installed, plan, guard, func(intent guestStorageLegacyChannelIntent, checkIntent func(context.Context) error) error {
		record := guestStorageChannelStage{Plan: plan, BootID: intent.BootID, RuntimeDevice: intent.RuntimeDevice, RuntimeInode: intent.RuntimeInode, Device: intent.Device, Inode: intent.Inode}
		return e.withRecordedGuestStorageChannelDirectory(ctx, record, true, checkIntent, func(root *os.Root, parent, candidate *os.File, check func(context.Context) error) error {
			if err := check(ctx); err != nil {
				return err
			}
			if _, err := root.Lstat(".homenode-guests.legacy"); os.IsNotExist(err) {
				if err := unix.Renameat2(int(parent.Fd()), "guests", int(parent.Fd()), ".homenode-guests.legacy", unix.RENAME_NOREPLACE); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			if err := candidate.Sync(); err != nil {
				return err
			}
			if err := parent.Sync(); err != nil {
				return err
			}
			if e.checkpoint != nil {
				if err := e.checkpoint("guest-storage-legacy-channel-archived", "run/homenode/.homenode-guests.legacy"); err != nil {
					return err
				}
			}
			if err := check(ctx); err != nil {
				return err
			}
			var archived unix.Stat_t
			if unix.Fstatat(int(parent.Fd()), ".homenode-guests.legacy", &archived, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(archived.Dev) != intent.Device || archived.Ino != intent.Inode || archived.Mode != unix.S_IFDIR|0755 || archived.Uid != 0 || archived.Gid != 0 {
				return ErrConflict
			}
			if _, err := root.Lstat("guests"); !os.IsNotExist(err) {
				return errors.Join(ErrConflict, err)
			}
			return check(ctx)
		})
	})
}
