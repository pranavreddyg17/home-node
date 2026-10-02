package backup

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// RunAdmittedLaunchedMaintenance owns admission and app restoration. Runtime
// acquisition and release belong exclusively to the isolated backup worker.
// Delivery callbacks must send once and await the exact authenticated completion;
// failures retain barriers for explicit reconciliation. The caller owns credential.
func RunAdmittedLaunchedMaintenance(ctx context.Context, store *state.Store, device, token, jobID, release string, catalogVersion int64, credential *os.File, apps MaintenanceApps, launchWorker func(context.Context, Launch, *os.File) error, cleanupWorker func(context.Context, Cleanup, *os.File) error) (snapshotID string, resultErr error) {
	launch := Launch{Version: 2, JobID: jobID, DeviceID: device, ManagementToken: token, Release: release, CatalogVersion: catalogVersion}
	if store == nil || apps == nil || credential == nil || launchWorker == nil || cleanupWorker == nil {
		return "", ErrManifest
	}
	if _, err := EncodeLaunch(launch); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	password, err := ReadRepositoryPassword(ctx, credential)
	if err != nil {
		return "", err
	}
	clear(password)
	runner, err := claimMaintenanceRunner(ctx, store)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, runner.Close()) }()
	if _, err = store.InspectAdmittedMaintenanceJob(ctx, token, jobID, device); err != nil {
		return "", err
	}
	defer func() {
		if resultErr == nil {
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		current, inspectErr := store.InspectMaintenanceJob(recovery, token)
		if inspectErr == nil && current.ID == jobID && current.Phase != "requires-action" {
			inspectErr = store.AdvanceMaintenanceJob(recovery, token, jobID, current.Phase, "requires-action")
		}
		resultErr = errors.Join(resultErr, inspectErr)
	}()
	if err = apps.DrainMaintenance(ctx, token, device); err != nil {
		return "", err
	}
	if err = store.AdvanceMaintenanceJob(ctx, token, jobID, "draining", "freezing"); err != nil {
		return "", err
	}
	if err = store.ClaimBackupLaunch(ctx, token, jobID); err != nil {
		return "", err
	}
	if err = launchWorker(ctx, launch, credential); err != nil {
		return "", err
	}
	current, _, err := store.InspectBackupOutcomes(ctx)
	if err != nil {
		return "", err
	}
	owned, err := store.InspectMaintenanceJob(ctx, token)
	if err != nil || owned.ID != jobID || owned.Device != device || owned.RootToken == "" || owned.Phase != "publishing" || current == nil || current.JobID != jobID || current.Status != "published" {
		return "", errors.Join(ErrBackupPublicationEvidence, err)
	}
	snapshotID = current.SnapshotID
	if err = store.RecordBackupWorkerCompleted(ctx, token, jobID); err != nil {
		return snapshotID, err
	}
	// Once the worker has stopped, restoration has a bounded context independent
	// of cancellation of the initiating request/task.
	cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err = store.AdvanceMaintenanceJob(cleanup, token, jobID, "publishing", "restoring"); err != nil {
		return snapshotID, err
	}
	request := Cleanup{Version: 3, JobID: jobID, DeviceID: device, ManagementToken: token, RuntimeToken: owned.RootToken}
	if err = cleanupWorker(cleanup, request, credential); err != nil {
		return snapshotID, err
	}
	restored, err := store.InspectMaintenanceJob(cleanup, token)
	if err != nil || restored.ID != jobID || restored.Device != device || restored.Phase != "restoring" || restored.RootToken != "" {
		return snapshotID, errors.Join(state.ErrMaintenanceOwner, err)
	}
	if err = store.RequireBackupWorkerStopped(cleanup, token, jobID); err != nil {
		return snapshotID, err
	}
	if err = apps.RestoreMaintenanceApps(cleanup, token, device); err != nil {
		return snapshotID, err
	}
	return snapshotID, store.CompleteMaintenanceJob(cleanup, token, jobID)
}
