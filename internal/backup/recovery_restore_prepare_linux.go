//go:build linux

package backup

import (
	"context"
	"errors"
	"os"
	"time"
)

// RestoreAndPreparePrivateRecovery extracts an exact encrypted snapshot into a
// newly created leased job directory and prepares disconnected replacement data.
// The caller must retain JobStaging until this call returns; Close never removes
// recovered data. Destination remains private runner-owned staging, not a live
// controller location. Extraction success followed by preparation failure leaves
// validated source for explicit retry through PreparePrivateRecovery.
func RestoreAndPreparePrivateRecovery(ctx context.Context, repository *Repository, staging *JobStaging, destination *os.Root, snapshotID string, policy RestorePolicy) (name string, result error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if repository == nil || staging == nil || staging.Root() == nil || staging.lock == nil || destination == nil || !repositoryPattern.MatchString(snapshotID) {
		return "", ErrManifest
	}
	if _, err := staging.lock.Stat(); err != nil {
		return "", err
	}
	source := staging.Root()
	sourceInfo, sourceErr := source.Stat(".")
	destinationInfo, destinationErr := destination.Stat(".")
	if sourceErr != nil || destinationErr != nil || os.SameFile(sourceInfo, destinationInfo) {
		return "", ErrManifest
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	destinationLock, err := lockPrivateRunnerRoot(deadline, destination)
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, destinationLock.Close())
		if result != nil {
			name = ""
		}
	}()
	directory, err := source.Open(".")
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, directory.Close())
		if result != nil {
			name = ""
		}
	}()
	manifest, err := repository.Restore(deadline, snapshotID, directory, policy)
	if err != nil {
		return "", err
	}
	// Source's parent lease held by JobStaging protects extraction while empty;
	// the source maintenance lease protects subsequent preparation/retry writers.
	sourceLock, err := lockPrivateRunnerRoot(deadline, source)
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, sourceLock.Close())
		if result != nil {
			name = ""
		}
	}()
	return prepareRecoveryManagement(deadline, source, destination, snapshotID, manifest, policy)
}
