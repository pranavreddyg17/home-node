//go:build linux

package install

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Caller retains independently qualified installed/runtime/account authority.
// Publish only the recorded candidate; a completed retry synchronizes and
// requalifies the same final inode without rewriting its immutable receipt.
func (e *Engine) publishGuestStorageChannelParent(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32, stage guestStorageChannelStage, guard func(context.Context) error) error {
	return e.withRecordedGuestStorageChannel(ctx, plan, transferGID, stage, guard, func(root *os.Root, parent, candidate *os.File, check func(context.Context) error) error {
		if err := check(ctx); err != nil {
			return err
		}
		if _, err := root.Lstat("guests"); os.IsNotExist(err) {
			if err := unix.Renameat2(int(parent.Fd()), ".homenode-guests.stage", int(parent.Fd()), "guests", unix.RENAME_NOREPLACE); err != nil {
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
			if err := e.checkpoint("guest-storage-channel-parent-published", "run/homenode/guests"); err != nil {
				return err
			}
		}
		if err := check(ctx); err != nil {
			return err
		}
		var final unix.Stat_t
		if unix.Fstatat(int(parent.Fd()), "guests", &final, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(final.Dev) != stage.Device || final.Ino != stage.Inode || final.Mode != unix.S_IFDIR|0710 || final.Uid != 0 || final.Gid != transferGID {
			return ErrConflict
		}
		if _, err := root.Lstat(".homenode-guests.stage"); !os.IsNotExist(err) {
			return errors.Join(ErrConflict, err)
		}
		return check(ctx)
	})
}
