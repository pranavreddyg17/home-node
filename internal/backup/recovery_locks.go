package backup

import (
	"context"
	"errors"
	"os"
)

// lockRecoveryRoots retains the same persistent kernel lock inode used by other
// maintenance writers on each private runner-owned root. Nonblocking acquisition
// refuses overlap; it is not protection against an independent privileged writer.
func lockRecoveryRoots(ctx context.Context, source, destination *os.Root) (sourceLock, destinationLock *os.File, result error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if source == nil || destination == nil {
		return nil, nil, ErrManifest
	}
	sourceInfo, sourceErr := source.Stat(".")
	destinationInfo, destinationErr := destination.Stat(".")
	if sourceErr != nil || destinationErr != nil || os.SameFile(sourceInfo, destinationInfo) {
		return nil, nil, ErrManifest
	}
	sourceLock, result = lockPrivateRunnerRoot(ctx, source)
	if result != nil {
		return nil, nil, result
	}
	destinationLock, result = lockPrivateRunnerRoot(ctx, destination)
	if result != nil {
		return nil, nil, errors.Join(result, sourceLock.Close())
	}
	return sourceLock, destinationLock, nil
}
