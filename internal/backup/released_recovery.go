package backup

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// RecoverReleasedBackupMaintenance retries app restoration only after durable
// publication, worker completion and backup-peer runtime release, or an owned
// stopped pre-acquisition repository refusal. It cannot
// acquire authority or replay uncertain worker cleanup/publication.
func RecoverReleasedBackupMaintenance(ctx context.Context, store *state.Store, token, id, device string, apps MaintenanceApps) (resultErr error) {
	if store == nil || apps == nil {
		return ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runner, err := claimMaintenanceRunner(ctx, store)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, runner.Close()) }()
	job, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil {
		return err
	}
	if job.ID != id || job.Device != device || job.RootToken != "" || (job.Phase != "restoring" && job.Phase != "requires-action") {
		return state.ErrMaintenanceOwner
	}
	if err = store.RequireBackupWorkerStopped(ctx, token, id); err != nil {
		return err
	}
	observation, err := store.InspectBackupObservation(ctx)
	outcome := observation.Current
	if err != nil || (observation.WorkerCompletion != "refused" && (observation.WorkerCompletion != "complete" || outcome == nil || outcome.JobID != id || outcome.Status != "published")) {
		return errors.Join(ErrBackupPublicationEvidence, err)
	}
	if err := store.Transaction(ctx, func(tx *sql.Tx) error {
		authority, current, err := state.AuthorizeReleasedBackupRecoveryTx(tx, device, id)
		if err != nil {
			return err
		}
		if authority != token || current != job {
			return state.ErrMaintenanceOwner
		}
		return nil
	}); err != nil {
		return err
	}
	// Record a failed/cancelled restoration independently of the work context.
	// It retains admission and exact completed-worker/released-root evidence.
	defer func() {
		if resultErr == nil {
			return
		}
		checkpoint, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		current, err := store.InspectMaintenanceJob(checkpoint, token)
		if err == nil && current.ID == id && current.Device == device && current.Phase == "restoring" && current.RootToken == "" {
			err = store.AdvanceMaintenanceJob(checkpoint, token, id, "restoring", "requires-action")
		}
		resultErr = errors.Join(resultErr, err)
	}()
	restoration, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if job.Phase != "restoring" {
		if err = store.AdvanceMaintenanceJob(restoration, token, id, job.Phase, "restoring"); err != nil {
			return err
		}
	}
	if err = apps.RestoreMaintenanceApps(restoration, token, device); err != nil {
		return err
	}
	return store.CompleteMaintenanceJob(restoration, token, id)
}
