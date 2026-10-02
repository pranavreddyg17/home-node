package state

import "database/sql"

// AuthorizeReleasedBackupRecoveryTx qualifies controller-internal authority in
// the transaction consuming owner approval. Returned tokens must remain private
// and cannot be used until commit. No runtime or app effects occur here.
func AuthorizeReleasedBackupRecoveryTx(tx *sql.Tx, device, id string) (string, MaintenanceJob, error) {
	if tx == nil || !recoveryInstanceID.MatchString(device) || !recoveryInstanceID.MatchString(id) {
		return "", MaintenanceJob{}, ErrMaintenanceOwner
	}
	job, err := readMaintenanceJob(tx)
	if err != nil || job.ID != id || job.Device != device || job.RootToken != "" || (job.Phase != "requires-action" && job.Phase != "restoring") {
		return "", MaintenanceJob{}, ErrMaintenanceOwner
	}
	if err = maintenanceDevice(tx, device); err != nil {
		return "", MaintenanceJob{}, err
	}
	var completion string
	if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", backupDispatchKey).Scan(&completion); err != nil || (completion != "complete:"+id && completion != "refused:"+id) {
		return "", MaintenanceJob{}, ErrMaintenanceOwner
	}
	if completion == "complete:"+id {
		outcome, err := readBackupOutcome(tx, backupOutcomeKey)
		if err != nil || outcome.JobID != id || outcome.Status != "published" {
			return "", MaintenanceJob{}, ErrBackupPublication
		}
	} else {
		if err := requireNoBackupPublicationForJob(tx, id); err != nil {
			return "", MaintenanceJob{}, err
		}
	}
	var token string
	if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", maintenanceKey).Scan(&token); err != nil {
		return "", MaintenanceJob{}, ErrMaintenanceOwner
	}
	if err = RequireMaintenanceOwner(tx, token); err != nil {
		return "", MaintenanceJob{}, err
	}
	return token, job, nil
}
