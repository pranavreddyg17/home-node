package backup

import (
	"context"
	"errors"
	"os"
)

// publishRecoveryManagement publishes only into an exclusively owned private
// disconnected recovery directory. A live controller must not open this database
// until installed ownership, disk policy and trusted bootstrap are reconstructed.
// Retained staging establishes same-inode proof for interrupted publication.
func publishRecoveryManagement(ctx context.Context, root *os.Root) (name string, result error) {
	receipt, err := reconcileRecoveryManagementOutput(ctx, root)
	if err != nil {
		return "", err
	}
	const stage = ".recovery-management.stage"
	const final = "management.db"
	expected, err := root.Lstat(stage)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 || expected.Size() != receipt.Output.Bytes {
		return "", ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if existing, statErr := root.Lstat(final); statErr == nil {
		if !existing.Mode().IsRegular() || !os.SameFile(expected, existing) {
			return "", ErrManifest
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	} else if err = root.Link(stage, final); err != nil {
		return "", err
	}
	directory, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, directory.Close())
		if result != nil {
			name = ""
		}
	}()
	if err = directory.Sync(); err != nil {
		return "", err
	}
	current, err := root.Lstat(final)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) || current.Size() != receipt.Output.Bytes || current.Mode().Perm() != 0600 {
		return "", ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return final, nil
}
