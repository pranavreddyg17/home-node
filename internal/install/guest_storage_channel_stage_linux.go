//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type guestStorageChannelStage struct {
	Version       int                          `json:"version"`
	Plan          GuestStorageProvisioningPlan `json:"plan"`
	TransferGID   uint32                       `json:"transferGid"`
	RuntimeDevice uint64                       `json:"runtimeDevice"`
	RuntimeInode  uint64                       `json:"runtimeInode"`
	Device        uint64                       `json:"device"`
	Inode         uint64                       `json:"inode"`
}

// Caller holds qualified installed/runtime/account authority. Create only a
// private candidate and retain its inode before recording ownership provenance.
// Publication is separate; uncertain partial candidates are never removed or
// adopted merely because their metadata matches.
func (e *Engine) stageGuestStorageChannelParent(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32, guard func(context.Context) error) (stage guestStorageChannelStage, result error) {
	if err := ctx.Err(); err != nil {
		return stage, err
	}
	if transferGID == 0 || transferGID > 1<<31-1 || transferGID == plan.GuestGID || guard == nil {
		return stage, ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return stage, err
	}
	result = e.withGuestStorageChannelRuntime(ctx, guard, func(root *os.Root, parent *os.File, checkRuntime func(context.Context) error) (result error) {
		if _, err := root.Lstat("guests"); !os.IsNotExist(err) {
			return errors.Join(ErrConflict, err)
		}
		if err := checkRuntime(ctx); err != nil {
			return err
		}
		const name = ".homenode-guests.stage"
		if err := root.Mkdir(name, 0700); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, file.Close()) }()
		var original, runtime unix.Stat_t
		if unix.Fstat(int(file.Fd()), &original) != nil || original.Mode != unix.S_IFDIR|0700 || original.Uid != 0 || original.Gid != 0 || unix.Fstat(int(parent.Fd()), &runtime) != nil || original.Dev != runtime.Dev || original.Ino == runtime.Ino {
			return ErrConflict
		}
		check := func() error {
			if err := checkRuntime(ctx); err != nil {
				return err
			}
			var current, named unix.Stat_t
			if unix.Fstat(int(file.Fd()), &current) != nil || unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != original.Dev || current.Ino != original.Ino || current.Uid != 0 || named.Dev != current.Dev || named.Ino != current.Ino || named.Mode != current.Mode || named.Uid != current.Uid || named.Gid != current.Gid {
				return ErrConflict
			}
			if !(current.Mode == unix.S_IFDIR|0700 && (current.Gid == 0 || current.Gid == transferGID) || current.Mode == unix.S_IFDIR|0710 && current.Gid == transferGID) {
				return ErrConflict
			}
			for _, attribute := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
				if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
					return ErrConflict
				}
			}
			var childMount, parentMount unix.Statx_t
			flags := unix.AT_EMPTY_PATH | unix.AT_SYMLINK_NOFOLLOW
			if unix.Statx(int(file.Fd()), "", flags, unix.STATX_MNT_ID, &childMount) != nil || unix.Statx(int(parent.Fd()), "", flags, unix.STATX_MNT_ID, &parentMount) != nil || childMount.Mask&unix.STATX_MNT_ID == 0 || parentMount.Mask&unix.STATX_MNT_ID == 0 || childMount.Mnt_id != parentMount.Mnt_id {
				return ErrConflict
			}
			reader, err := root.Open(name)
			if err != nil {
				return err
			}
			entries, readErr := reader.ReadDir(1)
			closeErr := reader.Close()
			if len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
				return ErrConflict
			}
			if _, err := root.Lstat("guests"); !os.IsNotExist(err) {
				return errors.Join(ErrConflict, err)
			}
			return checkRuntime(ctx)
		}
		if err := check(); err != nil {
			return err
		}
		if err := unix.Fchown(int(file.Fd()), 0, int(transferGID)); err != nil {
			return err
		}
		if err := check(); err != nil {
			return err
		}
		if err := file.Chmod(0710); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := parent.Sync(); err != nil {
			return err
		}
		if err := check(); err != nil {
			return err
		}
		stage = guestStorageChannelStage{Version: 1, Plan: plan, TransferGID: transferGID, RuntimeDevice: uint64(runtime.Dev), RuntimeInode: runtime.Ino, Device: uint64(original.Dev), Inode: original.Ino}
		encoded, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		if err := e.commitImmutableGuestIntent(ctx, "guest-storage-channel-stage.json", "guest-storage-channel-stage", encoded); err != nil {
			return err
		}
		return check()
	})
	if result != nil {
		stage = guestStorageChannelStage{}
	}
	return stage, result
}
