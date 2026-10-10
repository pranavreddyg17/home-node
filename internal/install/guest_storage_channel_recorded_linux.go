//go:build linux

package install

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"reflect"
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
		return e.withRecordedGuestStorageChannelDirectory(ctx, stage, false, outer, use)
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
