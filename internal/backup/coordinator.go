package backup

import (
	"context"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type MaintenanceApps interface {
	DrainMaintenance(context.Context, string, string) error
	RestoreMaintenanceApps(context.Context, string, string) error
}
type MaintenanceRoot interface {
	BeginRuntimeMaintenanceForJob(context.Context, string) (string, error)
	EndRuntimeMaintenance(context.Context, string) error
}

// RunMaintenance connects the private maintenance journal to trusted app/root
// bridges. Work must qualify filesystem consistency and perform protected backup
// staging/publication. No public route calls this coordinator yet. Cleanup gets
// an independent bounded context even if the work context is cancelled. Failure
// to reconcile/restart retains admission and returns the cleanup error.
func RunMaintenance(ctx context.Context, store *state.Store, device string, apps MaintenanceApps, root MaintenanceRoot, stage, publish func(context.Context, string, string) error) (jobID string, resultErr error) {
	if store == nil || apps == nil || root == nil || stage == nil || publish == nil {
		return "", ErrManifest
	}
	token, job, err := store.BeginMaintenanceJob(ctx, device)
	if err != nil {
		return "", err
	}
	jobID = job.ID
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		err := finishMaintenance(cleanup, store, token, job.ID, device, apps, root)
		if err != nil {
			if current, inspectErr := store.InspectMaintenanceJob(cleanup, token); inspectErr == nil && current.Phase != "requires-action" && current.Phase != "freezing" {
				err = errors.Join(err, store.AdvanceMaintenanceJob(cleanup, token, job.ID, current.Phase, "requires-action"))
			}
		}
		resultErr = errors.Join(resultErr, err)
	}()
	if err = apps.DrainMaintenance(ctx, token, device); err != nil {
		return jobID, err
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		return jobID, err
	}
	rootToken, err := root.BeginRuntimeMaintenanceForJob(ctx, job.ID)
	if err != nil {
		return jobID, err
	}
	if err = store.AttachMaintenanceRoot(ctx, token, job.ID, rootToken); err != nil {
		return jobID, err
	}
	if err = stage(ctx, token, rootToken); err != nil {
		return jobID, err
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "staging", "publishing"); err != nil {
		return jobID, err
	}
	return jobID, publish(ctx, token, rootToken)
}

func finishMaintenance(ctx context.Context, store *state.Store, token, id, device string, apps MaintenanceApps, root MaintenanceRoot) error {
	job, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil {
		return err
	}
	// A freezing checkpoint can precede a lost acquisition acknowledgement.
	// Retrying its stable job ID reconciles that acquisition before release.
	if job.Phase == "freezing" && job.RootToken == "" {
		rootToken, err := root.BeginRuntimeMaintenanceForJob(ctx, id)
		if err != nil {
			return err
		}
		if err = store.AttachMaintenanceRoot(ctx, token, id, rootToken); err != nil {
			return err
		}
		job, err = store.InspectMaintenanceJob(ctx, token)
		if err != nil {
			return err
		}
	}
	if job.Phase != "restoring" {
		if err = store.AdvanceMaintenanceJob(ctx, token, id, job.Phase, "restoring"); err != nil {
			return err
		}
	}
	if job.RootToken != "" {
		if err = store.ReleaseMaintenanceRoot(ctx, token, id, root.EndRuntimeMaintenance); err != nil {
			return err
		}
	}
	if err = apps.RestoreMaintenanceApps(ctx, token, device); err != nil {
		return err
	}
	return store.CompleteMaintenanceJob(ctx, token, id)
}

// RecoverMaintenance reconciles cleanup after the prior runner has stopped.
// It never repeats uncertain staging/publication. The trusted caller must
// ensure exclusive runner ownership; production restart claiming is not wired.
func RecoverMaintenance(ctx context.Context, store *state.Store, token, id string, apps MaintenanceApps, root MaintenanceRoot) error {
	if store == nil || apps == nil || root == nil {
		return ErrManifest
	}
	job, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil {
		return err
	}
	if job.ID != id {
		return state.ErrMaintenanceOwner
	}
	deadline, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	err = finishMaintenance(deadline, store, token, id, job.Device, apps, root)
	if err != nil {
		if current, inspectErr := store.InspectMaintenanceJob(deadline, token); inspectErr == nil && current.Phase != "requires-action" && current.Phase != "freezing" {
			err = errors.Join(err, store.AdvanceMaintenanceJob(deadline, token, id, current.Phase, "requires-action"))
		}
	}
	return err
}
