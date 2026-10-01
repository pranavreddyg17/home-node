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
