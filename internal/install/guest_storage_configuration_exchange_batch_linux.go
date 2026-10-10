//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
)

// Caller retains the independently authenticated intent, stage receipt,
// installation journal, directory pathname and dormant-runtime/account guards.
// Preflight both file pairs before any chmod or exchange. A retry accepts only
// recorded inode pairs, including the interruption between the two exchanges.
// Saving the destination installation journal and activation are separate.
func exchangeGuestStorageConfigurationBatch(ctx context.Context, directory *os.File, stage guestStorageConfigurationStage, intent guestStorageConfigurationIntent, guard func(context.Context) error) error {
	return reconcileGuestStorageConfigurationBatch(ctx, directory, stage, intent, guard, true)
}

func reconcileGuestStorageConfigurationBatch(ctx context.Context, directory *os.File, stage guestStorageConfigurationStage, intent guestStorageConfigurationIntent, guard func(context.Context) error, publish bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || guard == nil || stage.Version != 1 || intent.Version != 1 || len(stage.Files) != 2 {
		return ErrPlan
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if stage.IntentSHA256 != digest(encoded) || stage.Files[0].Name != ".homenode-runtime-policy.stage" || stage.Files[1].Name != ".homenode-services-env.stage" {
		return ErrConflict
	}
	first, second := stage.Files[0], stage.Files[1]
	if first.Device != second.Device || first.Inode == second.Inode || first.SourceInode == second.SourceInode || first.Inode == second.SourceInode || second.Inode == first.SourceInode {
		return ErrConflict
	}
	originals := [][]byte{intent.SourcePolicy, intent.SourceEnvironment}
	desired := [][]byte{intent.Policy, intent.Environment}
	preflight := func(ctx context.Context) error {
		if err := guard(ctx); err != nil {
			return err
		}
		for i, receipt := range stage.Files {
			if err := reconcileGuestStorageConfigurationFile(ctx, directory, receipt, originals[i], desired[i], guard, false); err != nil {
				return err
			}
		}
		return guard(ctx)
	}
	if err := preflight(ctx); err != nil {
		return err
	}
	if !publish {
		return ctx.Err()
	}
	for i, receipt := range stage.Files {
		if err := exchangeGuestStorageConfigurationFile(ctx, directory, receipt, originals[i], desired[i], preflight); err != nil {
			return err
		}
	}
	return preflight(ctx)
}
