//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Retain the legacy receipt alongside its authenticated installed policy. The
// consumer must additionally retain the recorded runtime/channel directories;
// a saved receipt alone never authorizes adoption or archival of a host inode.
func (e *Engine) withGuestStorageLegacyChannelIntent(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error, use func(guestStorageLegacyChannelIntent, func(context.Context) error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || e.journalRoot == nil || guard == nil || use == nil {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	policy, err := e.readGuestStorageConfigurationCandidate(ctx, installed, "runtime-policy.json", 0600)
	if err != nil {
		return err
	}
	if _, _, err := planGuestStorageRuntimePolicy(ctx, installed, policy, plan); err != nil {
		return err
	}
	const name = "guest-storage-legacy-channel-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	before, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, 8192) || !e.accountJournalPathUnchanged(name, before, 8192) {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return err
	}
	var intent guestStorageLegacyChannelIntent
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil {
		return ErrConflict
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) || intent.Version != 1 || !reflect.DeepEqual(intent.Plan, plan) || !guestStorageBootIDAdmitted(intent.BootID) || intent.PolicySHA256 != digest(policy) || intent.Device != intent.RuntimeDevice || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 || intent.RuntimeInode == 0 || intent.RuntimeInode > math.MaxInt64 || intent.Inode == intent.RuntimeInode {
		return ErrConflict
	}
	return e.withGuestStorageConfigurationSource(ctx, "runtime-policy.json", string(policy), func(ctx context.Context, _ *os.File, checkPolicy func() error) error {
		return e.withGuestIdentityRecordGuarded(ctx, name, canonical, func(ctx context.Context, checkReceipt func() error) error {
			check := func(ctx context.Context) error {
				if err := guard(ctx); err != nil {
					return err
				}
				if err := checkPolicy(); err != nil {
					return err
				}
				if err := checkReceipt(); err != nil {
					return err
				}
				if !e.accountJournalPathUnchanged(name, before, 8192) {
					return ErrConflict
				}
				current, err := io.ReadAll(io.NewSectionReader(file, 0, 8193))
				if err != nil {
					return err
				}
				if !bytes.Equal(current, canonical) {
					return ErrConflict
				}
				bootID, err := observeGuestStorageBootID(ctx)
				if err != nil {
					return err
				}
				if bootID != intent.BootID {
					return ErrConflict
				}
				return guard(ctx)
			}
			if err := check(ctx); err != nil {
				return err
			}
			if err := use(intent, check); err != nil {
				return err
			}
			return check(ctx)
		})
	})
}
