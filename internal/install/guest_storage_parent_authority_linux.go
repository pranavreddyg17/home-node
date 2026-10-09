//go:build linux

package install

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
)

type guestStorageParentAuthority struct {
	Plan       GuestStorageProvisioningPlan
	Images     guestStorageImagesIntent
	Parent     guestStorageImageParentIntent
	Transition guestStorageParentJournalIntent
}

// Caller holds e.mu. No descriptor or authority escapes this retained scope.
func (e *Engine) withGuestStorageParentAuthorityLocked(ctx context.Context, use func(context.Context, guestStorageParentAuthority, *os.Root, *os.File, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	_, err := e.withGuestStorageIntentGuarded(ctx, func(ctx context.Context, plan GuestStorageProvisioningPlan, checkPlan func() error) error {
		return e.withGuestStorageImagesIntentGuarded(ctx, func(ctx context.Context, images guestStorageImagesIntent, checkImages func() error) error {
			if !reflect.DeepEqual(images.Plan, plan) || len(images.Images) != 3 {
				return ErrConflict
			}
			sourceGID := images.Images[0].SourceGID
			return e.withGuestStorageImageParentIntent(ctx, images, sourceGID, func(parent guestStorageImageParentIntent, checkParent func() error) error {
				encoded, err := json.Marshal(parent)
				if err != nil {
					return err
				}
				current, err := e.load()
				if err != nil {
					return err
				}
				return e.withGuestStorageParentJournalIntent(ctx, current, sourceGID, plan.GuestGID, digest(encoded), func(transition guestStorageParentJournalIntent, checkTransition func() error) error {
					provenance := func(ctx context.Context) error {
						if err := ctx.Err(); err != nil {
							return err
						}
						for _, check := range []func() error{checkPlan, checkImages, checkParent, checkTransition} {
							if err := check(); err != nil {
								return err
							}
						}
						return ctx.Err()
					}
					return e.withRecordedGuestStorageImageParent(ctx, parent, provenance, func(root *os.Root, file *os.File, checkPath func(context.Context) error) error {
						authority := guestStorageParentAuthority{Plan: plan, Images: images, Parent: parent, Transition: transition}
						return use(ctx, authority, root, file, checkPath)
					})
				})
			})
		})
	})
	return err
}
