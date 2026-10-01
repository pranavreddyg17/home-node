package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

type ManagementPublication interface {
	BeginPublishing(context.Context, string, string) error
	ConfirmPublishing(context.Context, string, string) error
	ClaimPublication(context.Context, string, string) error
}

// PublishPrivateRecoverySet writes an already staged recovery set while the
// caller retains owned maintenance and exclusive repository/staging ownership.
// A returned snapshot ID survives a subsequent confirmation failure; callers
// must persist that distinction rather than reporting successful cleanup.
func PublishPrivateRecoverySet(ctx context.Context, management ManagementPublication, repository RecoveryPublisher, device, token string, root *os.Root, manifest Manifest, policy RestorePolicy) (snapshotID string, resultErr error) {
	if management == nil || repository == nil || root == nil || !guestproto.ValidID(device) || !guestproto.ValidID(token) {
		return "", ErrManifest
	}
	directory, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer directory.Close()
	if err = ValidateRecoverySet(ctx, root, manifest, policy); err != nil {
		return "", err
	}
	if err = management.BeginPublishing(ctx, token, device); err != nil {
		return "", err
	}
	if err = management.ConfirmPublishing(ctx, token, device); err != nil {
		return "", err
	}
	if err = management.ClaimPublication(ctx, token, device); err != nil {
		return "", err
	}
	snapshotID, err = repository.Snapshot(ctx, directory, manifest, policy)
	if err != nil {
		return snapshotID, err
	}
	if !repositoryPattern.MatchString(snapshotID) {
		return "", ErrRepository
	}
	return snapshotID, management.ConfirmPublishing(ctx, token, device)
}
