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

type guestStorageLegacyChannelIntent struct {
	Version       int                          `json:"version"`
	Plan          GuestStorageProvisioningPlan `json:"plan"`
	BootID        string                       `json:"bootId"`
	PolicySHA256  string                       `json:"policySha256"`
	RuntimeDevice uint64                       `json:"runtimeDevice"`
	RuntimeInode  uint64                       `json:"runtimeInode"`
	Device        uint64                       `json:"device"`
	Inode         uint64                       `json:"inode"`
}

// Caller retains qualified installed/runtime/account exclusion. Record a legacy
// empty channel only while the journaled policy still selects legacy behavior.
// This does not alter, archive or adopt the directory for reserved execution.
func (e *Engine) prepareGuestStorageLegacyChannelIntent(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || guard == nil {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	policy, err := e.readGuestStorageConfigurationCandidate(ctx, installed, "runtime-policy.json", 0600)
	if err != nil {
		return err
	}
	// Reuse the strict authenticated planner to require a valid legacy policy,
	// service identities and installed journal. Discard the proposed publication.
	if _, _, err := planGuestStorageRuntimePolicy(ctx, installed, policy, plan); err != nil {
		return err
	}
	bootID, err := observeGuestStorageBootID(ctx)
	if err != nil {
		return err
	}
	return e.withGuestStorageConfigurationSource(ctx, "runtime-policy.json", string(policy), func(ctx context.Context, _ *os.File, checkPolicy func() error) error {
		outer := func(ctx context.Context) error {
			if err := checkPolicy(); err != nil {
				return err
			}
			return guard(ctx)
		}
		return e.withGuestStorageChannelRuntime(ctx, outer, func(root *os.Root, parent *os.File, checkRuntime func(context.Context) error) (result error) {
			file, err := root.OpenFile("guests", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, file.Close()) }()
			var initial, runtime unix.Stat_t
			if unix.Fstat(int(file.Fd()), &initial) != nil || initial.Mode != unix.S_IFDIR|0755 || initial.Uid != 0 || initial.Gid != 0 || unix.Fstat(int(parent.Fd()), &runtime) != nil || initial.Dev != runtime.Dev || initial.Ino == runtime.Ino {
				return ErrConflict
			}
			check := func() error {
				if err := checkRuntime(ctx); err != nil {
					return err
				}
				observedBoot, err := observeGuestStorageBootID(ctx)
				if err != nil {
					return err
				}
				if observedBoot != bootID {
					return ErrConflict
				}
				var current, named unix.Stat_t
				if unix.Fstat(int(file.Fd()), &current) != nil || unix.Fstatat(int(parent.Fd()), "guests", &named, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != initial.Dev || current.Ino != initial.Ino || current.Mode != initial.Mode || current.Uid != 0 || current.Gid != 0 || named.Dev != current.Dev || named.Ino != current.Ino || named.Mode != current.Mode || named.Uid != current.Uid || named.Gid != current.Gid {
					return ErrConflict
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
				reader, err := root.Open("guests")
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
				return checkRuntime(ctx)
			}
			if err := check(); err != nil {
				return err
			}
			intent := guestStorageLegacyChannelIntent{Version: 1, Plan: plan, BootID: bootID, PolicySHA256: digest(policy), RuntimeDevice: uint64(runtime.Dev), RuntimeInode: runtime.Ino, Device: uint64(initial.Dev), Inode: initial.Ino}
			encoded, err := json.Marshal(intent)
			if err != nil {
				return err
			}
			if err := e.commitImmutableGuestIntent(ctx, "guest-storage-legacy-channel-intent.json", "guest-storage-legacy-channel", encoded); err != nil {
				return err
			}
			return check()
		})
	})
}
