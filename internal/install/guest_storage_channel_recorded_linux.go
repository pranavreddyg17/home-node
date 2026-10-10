//go:build linux

package install

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Retain a supplied receipt against independently qualified allocation and
// transfer identity. Exactly one fixed location must contain its recorded inode.
// The consumer may atomically publish that inode, but never replace or adopt it.
func (e *Engine) withRecordedGuestStorageChannel(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32, stage guestStorageChannelStage, guard func(context.Context) error, use func(*os.Root, *os.File, *os.File, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || e.journalRoot == nil || guard == nil || use == nil {
		return ErrPlan
	}
	if err := validateGuestStorageChannelStage(ctx, plan, transferGID, stage); err != nil {
		return err
	}
	encoded, err := json.Marshal(stage)
	if err != nil {
		return err
	}
	return e.withGuestIdentityRecordGuarded(ctx, "guest-storage-channel-stage.json", encoded, func(ctx context.Context, checkReceipt func() error) error {
		outer := func(ctx context.Context) error {
			bootID, err := observeGuestStorageBootID(ctx)
			if err != nil {
				return err
			}
			if bootID != stage.BootID {
				return ErrConflict
			}
			if err := checkReceipt(); err != nil {
				return err
			}
			return guard(ctx)
		}
		return e.withGuestStorageChannelRuntime(ctx, outer, func(root *os.Root, parent *os.File, checkRuntime func(context.Context) error) (result error) {
			var runtime unix.Stat_t
			if unix.Fstat(int(parent.Fd()), &runtime) != nil || uint64(runtime.Dev) != stage.RuntimeDevice || runtime.Ino != stage.RuntimeInode {
				return ErrConflict
			}
			location := func() (string, error) {
				selected := ""
				for _, name := range []string{".homenode-guests.stage", "guests"} {
					var observed unix.Stat_t
					err := unix.Fstatat(int(parent.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW)
					if errors.Is(err, unix.ENOENT) {
						continue
					}
					if err != nil || selected != "" || uint64(observed.Dev) != stage.Device || observed.Ino != stage.Inode || observed.Mode != unix.S_IFDIR|0710 || observed.Uid != 0 || observed.Gid != transferGID {
						return "", ErrConflict
					}
					selected = name
				}
				if selected == "" {
					return "", ErrConflict
				}
				return selected, nil
			}
			name, err := location()
			if err != nil {
				return err
			}
			file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, file.Close()) }()
			check := func(ctx context.Context) error {
				if err := checkRuntime(ctx); err != nil {
					return err
				}
				var current unix.Stat_t
				if unix.Fstat(int(file.Fd()), &current) != nil || uint64(current.Dev) != stage.Device || current.Ino != stage.Inode || current.Mode != unix.S_IFDIR|0710 || current.Uid != 0 || current.Gid != transferGID {
					return ErrConflict
				}
				name, err := location()
				if err != nil {
					return err
				}
				for _, attr := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
					if _, err := unix.Fgetxattr(int(file.Fd()), attr, nil); !errors.Is(err, unix.ENODATA) {
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
				observed, statErr := reader.Stat()
				retained, retainedErr := file.Stat()
				entries, readErr := reader.ReadDir(1)
				closeErr := reader.Close()
				if statErr != nil || retainedErr != nil || !os.SameFile(observed, retained) || len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
					return ErrConflict
				}
				if _, err := location(); err != nil {
					return err
				}
				return checkRuntime(ctx)
			}
			if err := check(ctx); err != nil {
				return err
			}
			if err := use(root, parent, file, check); err != nil {
				return err
			}
			return check(ctx)
		})
	})
}

func validateGuestStorageChannelStage(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32, stage guestStorageChannelStage) error {
	if stage.Version != 2 || !guestStorageBootIDAdmitted(stage.BootID) || transferGID == 0 || transferGID > math.MaxInt32 || transferGID == plan.GuestGID || stage.TransferGID != transferGID || !reflect.DeepEqual(stage.Plan, plan) || stage.Device != stage.RuntimeDevice || stage.Device > math.MaxInt64 || stage.Inode == 0 || stage.Inode > math.MaxInt64 || stage.RuntimeInode == 0 || stage.RuntimeInode > math.MaxInt64 || stage.Inode == stage.RuntimeInode {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return err
	}
	return ctx.Err()
}
