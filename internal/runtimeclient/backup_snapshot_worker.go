package runtimeclient

import (
	"context"
	"errors"
	"os"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// SnapshotPageAuthority must verify the exact pending controller request and
// current owner/session authority through the protected management channel.
// A device identifier alone is insufficient. Verification must remain valid
// for both checks; it must not consume the request at the first check.
type SnapshotPageAuthority interface {
	VerifySnapshotPage(context.Context, backup.SnapshotPageRequest) error
}

// RunCredentialedSnapshotPage consumes the credential on every path. It opens
// only the installed registered target and never acquires runtime authority,
// creates staging, restores payloads or changes app state.
func RunCredentialedSnapshotPage(ctx context.Context, request backup.SnapshotPageRequest, config BackupWorkerConfig, credential *os.File, authority SnapshotPageAuthority) (page backup.SnapshotPage, resultErr error) {
	if credential == nil {
		return page, backup.ErrRepository
	}
	defer func() {
		resultErr = errors.Join(resultErr, credential.Close())
		if resultErr != nil {
			page = backup.SnapshotPage{}
		}
	}()
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if _, err := backup.EncodeSnapshotPageRequest(request); err != nil {
		return page, err
	}
	if authority == nil {
		return page, backup.ErrManifest
	}
	if err := config.RepositoryTarget.Validate(); err != nil {
		return page, err
	}
	if err := authority.VerifySnapshotPage(ctx, request); err != nil {
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
	return authorizedSnapshotPage(ctx, request, authority, repository)
}

type snapshotPageReader interface {
	SnapshotPage(context.Context, string) (backup.SnapshotPage, error)
}

// The initial verification precedes repository opening; this second check
// prevents returning candidates after authority was withdrawn during listing.
func authorizedSnapshotPage(ctx context.Context, request backup.SnapshotPageRequest, authority SnapshotPageAuthority, repository snapshotPageReader) (backup.SnapshotPage, error) {
	page, err := repository.SnapshotPage(ctx, request.Cursor)
	if err != nil {
		return backup.SnapshotPage{}, err
	}
	if err := authority.VerifySnapshotPage(ctx, request); err != nil {
		return backup.SnapshotPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return backup.SnapshotPage{}, err
	}
	return page, nil
}
