//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
)

// Caller holds installer and allocation exclusion, a qualified host directory,
// independently derived ownership and a retained activation barrier. Both
// immutable records remain open across exchange and its reconciliation checks.
// This private composition does not activate a production installer command.
func (e *Engine) publishGuestIdentityNameServices(ctx context.Context, directory *os.File, stage guestIdentityNameServiceStage, intent guestIdentityNameServiceIntent, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if directory == nil || guard == nil || stage.Version != 1 || stage.Inode == 0 || stage.SourceInode == 0 || stage.Device != stage.SourceDevice || stage.Inode == stage.SourceInode || stage.Bytes != int64(len(intent.Proposal.Contents)) {
		return ErrPlan
	}
	encodedIntent, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if stage.IntentSHA256 != digest(encodedIntent) {
		return ErrConflict
	}
	encodedStage, err := json.Marshal(stage)
	if err != nil {
		return err
	}
	return e.withGuestIdentityNameServiceIntentGuarded(ctx, intent, func(ctx context.Context, checkIntent func() error) error {
		return e.withGuestIdentityRecordGuarded(ctx, "guest-identity-nss-stage.json", encodedStage, func(ctx context.Context, checkStage func() error) error {
			retainedGuard := func(ctx context.Context) error {
				if err := checkIntent(); err != nil {
					return err
				}
				if err := checkStage(); err != nil {
					return err
				}
				if err := guard(ctx); err != nil {
					return err
				}
				if err := checkIntent(); err != nil {
					return err
				}
				return checkStage()
			}
			return exchangeGuestIdentityConfiguration(ctx, directory, stage, []byte(intent.Original), []byte(intent.Proposal.Contents), retainedGuard)
		})
	})
}
