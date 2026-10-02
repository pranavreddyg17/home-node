package state

import (
	"context"
	"database/sql"
	"errors"
)

const backupDispatchKey = "host.backup-dispatch"

func requireBackupWorkerStopped(tx *sql.Tx, id string) error {
	var value string
	err := tx.QueryRow("SELECT value FROM settings WHERE key=?", backupDispatchKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if value == "refused:"+id {
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id || job.RootToken != "" || (job.Phase != "restoring" && job.Phase != "requires-action") {
			return ErrMaintenanceOwner
		}
		return maintenanceDevice(tx, job.Device)
	}
	if value != "complete:"+id {
		return ErrMaintenanceOwner
	}
	return nil
}

// RequireBackupWorkerStopped refuses cleanup after an attempted dispatch until
// its owned completion checkpoint is durable. Publication alone is insufficient.
func (s *Store) RequireBackupWorkerStopped(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id {
			return ErrMaintenanceOwner
		}
		return requireBackupWorkerStopped(tx, id)
	})
}

func (s *Store) ClaimBackupDispatch(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id || job.Phase != "staging" || job.RootToken == "" {
			return ErrMaintenanceOwner
		}
		if err = maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", backupDispatchKey, "uncertain:"+id)
		return err
	})
}

// RecordBackupWorkerCompleted is called only after the trusted exact completion
// response; it additionally requires owned durable published evidence atomically.
func (s *Store) RecordBackupWorkerCompleted(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id || job.Phase != "publishing" || job.RootToken == "" {
			return ErrMaintenanceOwner
		}
		if err = maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		outcome, err := readBackupOutcome(tx, backupOutcomeKey)
		if err != nil || outcome.JobID != id || outcome.Status != "published" {
			return ErrBackupPublication
		}
		var claim string
		if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", backupPublicationKey).Scan(&claim); err != nil || claim != id {
			return ErrBackupPublication
		}
		result, err := tx.Exec("UPDATE settings SET value=? WHERE key=? AND value=?", "complete:"+id, backupDispatchKey, "uncertain:"+id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return ErrMaintenanceOwner
		}
		return nil
	})
}

// BackupObservation reads publication and completion evidence in one snapshot.
// It exposes no maintenance tokens and does not attest repository/restore health.
type BackupObservation struct {
	Current          *BackupOutcome `json:"current"`
	LastPublished    *BackupOutcome `json:"lastPublished"`
	WorkerCompletion string         `json:"workerCompletion"`
}

func (s *Store) InspectBackupObservation(ctx context.Context) (observation BackupObservation, resultErr error) {
	observation.WorkerCompletion = "none"
	resultErr = s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := readBackupOutcomes(tx, &observation.Current, &observation.LastPublished); err != nil {
			return err
		}
		var raw string
		err := tx.QueryRow("SELECT value FROM settings WHERE key=?", backupDispatchKey).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		switch raw {
		case "uncertain:" + job.ID:
			if (job.Phase != "freezing" && job.Phase != "staging" && job.Phase != "publishing" && job.Phase != "requires-action") || (job.RootToken == "" && job.Phase != "freezing" && job.Phase != "requires-action") || (job.RootToken != "" && job.Phase == "freezing") {
				return ErrMaintenance
			}
			observation.WorkerCompletion = "uncertain"
		case "refused:" + job.ID:
			if job.RootToken != "" || (job.Phase != "restoring" && job.Phase != "requires-action") {
				return ErrMaintenance
			}
			observation.WorkerCompletion = "refused"
		case "complete:" + job.ID:
			if observation.Current == nil || observation.Current.JobID != job.ID || observation.Current.Status != "published" || (job.Phase != "publishing" && job.Phase != "restoring" && job.Phase != "requires-action") {
				return ErrMaintenance
			}
			observation.WorkerCompletion = "complete"
		default:
			return ErrMaintenance
		}
		return nil
	})
	if resultErr != nil {
		return BackupObservation{}, resultErr
	}
	return
}

// ClaimBackupLaunch durably records uncertainty before any preliminary worker
// handoff/runtime acquisition. It does not claim that a root token exists yet.
func (s *Store) ClaimBackupLaunch(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id || job.Phase != "freezing" || job.RootToken != "" {
			return ErrMaintenanceOwner
		}
		if err := maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		inventory, err := readMaintenanceInventory(tx)
		if err != nil || inventory != (MaintenanceInventory{}) {
			return ErrMaintenanceOwner
		}
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", backupDispatchKey, "uncertain:"+id)
		return err
	})
}

// RecordBackupLaunchRefused is called only after an authenticated exact stopped
// refusal reply. It atomically qualifies the preliminary owned checkpoint and
// moves to restoration; it never records publication or releases root authority.
func (s *Store) RecordBackupLaunchRefused(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil || job.ID != id || job.Phase != "freezing" || job.RootToken != "" {
			return ErrMaintenanceOwner
		}
		if err := maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		inventory, err := readMaintenanceInventory(tx)
		if err != nil || inventory != (MaintenanceInventory{}) {
			return ErrMaintenanceOwner
		}
		var claims int
		if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key=?", backupPublicationKey).Scan(&claims); err != nil {
			return err
		}
		if claims != 0 {
			return ErrBackupPublication
		}
		result, err := tx.Exec("UPDATE settings SET value=? WHERE key=? AND value=?", "refused:"+id, backupDispatchKey, "uncertain:"+id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return ErrMaintenanceOwner
		}
		if _, err := tx.Exec("UPDATE settings SET value='2' WHERE key=?", maintenanceJobPrefix+"version"); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE settings SET value='restoring' WHERE key=?", maintenanceJobPrefix+"phase")
		return err
	})
}
