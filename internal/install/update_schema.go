package install

import (
	"context"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/updates"
)

// The operation boundary observes actual state on both sides of acquisition.
// This consistency check does not replace exclusive migration admission.
func acquireWithObservedSchema(ctx context.Context, store *state.Store, acquire func(int) (*updates.AcquiredRelease, error)) (*updates.AcquiredRelease, error) {
	current, err := store.ObserveSchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	release, err := acquire(current)
	if err != nil {
		if release != nil && release.Package != nil {
			err = errors.Join(err, release.Close())
		}
		return nil, err
	}
	if release == nil || release.Package == nil {
		return nil, ErrConflict
	}
	observed, observationErr := store.ObserveSchemaVersion(ctx)
	if observationErr != nil || observed != current {
		return nil, errors.Join(ErrConflict, observationErr, release.Close())
	}
	return release, nil
}
