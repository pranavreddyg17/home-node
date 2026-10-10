//go:build linux

package install

import (
	"context"
	"errors"
	"os"
)

// Caller retains installed/runtime/account authority. Only current runtime
// entries trigger legacy migration; a dormant historic receipt on a later boot
// never authorizes adopting a new directory or prevents fresh empty staging.
func (e *Engine) migrateGuestStorageLegacyChannel(ctx context.Context, installed journal, plan GuestStorageProvisioningPlan, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || e.host == nil || e.journalRoot == nil || guard == nil {
		return ErrPlan
	}
	if err := guard(ctx); err != nil {
		return err
	}
	present := make([]bool, 2)
	for i, name := range []string{"run/homenode/guests", "run/homenode/.homenode-guests.legacy"} {
		if _, err := e.host.Lstat(name); err == nil {
			present[i] = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if !present[0] && !present[1] {
		return guard(ctx)
	}
	if present[0] && present[1] {
		return ErrConflict
	}
	if _, err := e.journalRoot.Lstat("guest-storage-legacy-channel-intent.json"); os.IsNotExist(err) {
		if present[1] {
			return ErrConflict
		}
		if err := e.prepareGuestStorageLegacyChannelIntent(ctx, installed, plan, guard); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return err
	}
	if err := e.archiveGuestStorageLegacyChannel(ctx, installed, plan, guard); err != nil {
		return errors.Join(ErrConflict, err)
	}
	return guard(ctx)
}
