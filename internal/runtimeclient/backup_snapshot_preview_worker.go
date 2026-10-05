package runtimeclient

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// SnapshotPreviewAuthority must verify the exact pending controller request and
// current owner/session authority through the protected management channel.
// A device identifier alone is insufficient. Verification must remain valid
// for both checks; it must not consume the request at the first check.
type SnapshotPreviewAuthority interface {
	VerifySnapshotPreview(context.Context, backup.SnapshotPreviewRequest) error
}

// RunCredentialedSnapshotPreview consumes the credential on every path. It opens
// only the installed registered target and never acquires runtime authority,
// creates staging, restores payloads or changes app state.
func RunCredentialedSnapshotPreview(ctx context.Context, request backup.SnapshotPreviewRequest, config BackupWorkerConfig, credential *os.File, authority SnapshotPreviewAuthority) (page backup.SnapshotPreview, resultErr error) {
	if credential == nil {
		return page, backup.ErrRepository
	}
	defer func() {
		resultErr = errors.Join(resultErr, credential.Close())
		if resultErr != nil {
			page = backup.SnapshotPreview{}
		}
	}()
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if _, err := backup.EncodeSnapshotPreviewRequest(request); err != nil {
		return page, err
	}
	if authority == nil || config.Policy.MinimumCatalogVersion < 1 {
		return page, backup.ErrManifest
	}
	if err := config.RepositoryTarget.Validate(); err != nil {
		return page, err
	}
	if err := authority.VerifySnapshotPreview(ctx, request); err != nil {
		return page, err
	}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	password, err := backup.ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return page, err
	}
	defer clear(password)
	repository, err := backup.OpenRepository(ctx, config.RepositoryTarget, password)
	clear(password)
	if err != nil {
		return page, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, repository.Close())
	}()
	return authorizedSnapshotPreview(ctx, request, authority, repository, config.Policy)
}

type snapshotPreviewReader interface {
	PreviewSnapshot(context.Context, string, backup.RestorePolicy) (backup.SnapshotPreview, error)
}

// The initial verification precedes repository opening; this second check
// prevents returning candidates after authority was withdrawn during listing.
func authorizedSnapshotPreview(ctx context.Context, request backup.SnapshotPreviewRequest, authority SnapshotPreviewAuthority, repository snapshotPreviewReader, policy backup.RestorePolicy) (backup.SnapshotPreview, error) {
	page, err := repository.PreviewSnapshot(ctx, request.SnapshotID, policy)
	if err != nil {
		return backup.SnapshotPreview{}, err
	}
	if err := authority.VerifySnapshotPreview(ctx, request); err != nil {
		return backup.SnapshotPreview{}, err
	}
	if err := ctx.Err(); err != nil {
		return backup.SnapshotPreview{}, err
	}
	return page, nil
}
