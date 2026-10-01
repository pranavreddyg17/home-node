package backup

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// RunDispatchedMaintenance owns controller admission and runtime barriers while
// an isolated credential worker stages and publishes. deliver must send once to
// the trusted activated listener and wait for its job-bound completion response.
// Once delivery is attempted, any uncertain result retains barriers for explicit
// reconciliation. The caller retains ownership of the sealed credential.
func RunDispatchedMaintenance(ctx context.Context, store *state.Store, device, release string, catalogVersion int64, credential *os.File, apps MaintenanceApps, root MaintenanceRoot, deliver func(context.Context, Dispatch, *os.File) error) (jobID, snapshotID string, resultErr error) {
	return runDispatchedMaintenance(ctx, store, device, "", "", release, catalogVersion, credential, apps, root, deliver)
}

// RunAdmittedDispatchedMaintenance takes over the draining intent committed
// with owner approval. It requires the exact admission token, job and device;
// advanced or uncertain jobs must use explicit recovery, never redispatch.
// Invalid ownership leaves the existing job and barriers untouched.
func RunAdmittedDispatchedMaintenance(ctx context.Context, store *state.Store, device, token, jobID, release string, catalogVersion int64, credential *os.File, apps MaintenanceApps, root MaintenanceRoot, deliver func(context.Context, Dispatch, *os.File) error) (string, string, error) {
	if token == "" || jobID == "" {
		return "", "", state.ErrMaintenanceOwner
	}
	return runDispatchedMaintenance(ctx, store, device, token, jobID, release, catalogVersion, credential, apps, root, deliver)
}

func runDispatchedMaintenance(ctx context.Context, store *state.Store, device, token, admittedJobID, release string, catalogVersion int64, credential *os.File, apps MaintenanceApps, root MaintenanceRoot, deliver func(context.Context, Dispatch, *os.File) error) (jobID, snapshotID string, resultErr error) {
	if store == nil || apps == nil || root == nil || deliver == nil || credential == nil || !releasePattern.MatchString(release) || catalogVersion < 1 {
		return "", "", ErrManifest
	}
	password, err := ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return "", "", err
	}
	clear(password)
	runner, err := claimMaintenanceRunner(ctx, store)
	if err != nil {
		return "", "", err
	}
	defer func() { resultErr = errors.Join(resultErr, runner.Close()) }()
	var job state.MaintenanceJob
	if admittedJobID == "" {
		token, job, err = store.BeginMaintenanceJob(ctx, device)
	} else {
		job, err = store.InspectAdmittedMaintenanceJob(ctx, token, admittedJobID, device)
	}
	if err != nil {
		return "", "", err
	}
	jobID = job.ID
	uncertain := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if uncertain {
			current, err := store.InspectMaintenanceJob(cleanup, token)
			if err == nil && current.Phase != "requires-action" {
				err = store.AdvanceMaintenanceJob(cleanup, token, job.ID, current.Phase, "requires-action")
			}
			resultErr = errors.Join(resultErr, err)
			return
		}
		err := finishMaintenance(cleanup, store, token, job.ID, device, apps, root)
		if err != nil {
			if current, inspectErr := store.InspectMaintenanceJob(cleanup, token); inspectErr == nil && current.Phase != "requires-action" && current.Phase != "freezing" {
				err = errors.Join(err, store.AdvanceMaintenanceJob(cleanup, token, job.ID, current.Phase, "requires-action"))
			}
		}
		resultErr = errors.Join(resultErr, err)
	}()
	if err = apps.DrainMaintenance(ctx, token, device); err != nil {
		return jobID, "", err
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, job.ID, "draining", "freezing"); err != nil {
		return jobID, "", err
	}
	rootToken, err := root.BeginRuntimeMaintenanceForJob(ctx, job.ID)
	if err != nil {
		return jobID, "", err
	}
	if err = store.AttachMaintenanceRoot(ctx, token, job.ID, rootToken); err != nil {
		return jobID, "", err
	}
	dispatch := Dispatch{Version: 1, JobID: job.ID, DeviceID: device, ManagementToken: token, RuntimeToken: rootToken, Release: release, CatalogVersion: catalogVersion}
	if _, err = EncodeDispatch(dispatch); err != nil {
		return jobID, "", err
	}
	if err = store.ClaimBackupDispatch(ctx, token, job.ID); err != nil {
		return jobID, "", err
	}
	uncertain = true
	if err = deliver(ctx, dispatch, credential); err != nil {
		return jobID, "", err
	}
	current, _, err := store.InspectBackupOutcomes(ctx)
	if err != nil {
		return jobID, "", err
	}
	owned, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil || owned.ID != job.ID || owned.RootToken != rootToken || owned.Phase != "publishing" || current == nil || current.JobID != job.ID || current.Status != "published" {
		return jobID, "", errors.Join(ErrBackupPublicationEvidence, err)
	}
	snapshotID = current.SnapshotID
	if err = store.RecordBackupWorkerCompleted(ctx, token, job.ID); err != nil {
		return jobID, snapshotID, err
	}
	uncertain = false
	return jobID, snapshotID, nil
}

var ErrBackupPublicationEvidence = errors.New("completed backup worker lacks matching durable publication evidence")
