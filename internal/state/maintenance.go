package state

import (
	"context"
	"database/sql"
	"errors"
)

var ErrMaintenance = errors.New("host maintenance blocks new work")
var ErrMaintenanceOwner = errors.New("maintenance token does not own the barrier")

const maintenanceKey = "host.maintenance"

// BeginMaintenance closes transactional workload admission before draining work.
// There is intentionally no timeout: a crash must not reopen admission while
// disks may still be stopped or undergoing backup/restore. This barrier alone
// does not prove that in-flight operations are drained or disks are stable.
func (s *Store) BeginMaintenance(ctx context.Context) (string, error) {
	token := Random()
	err := s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireAdmission(tx); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", maintenanceKey, token)
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// EndMaintenance only releases the caller's barrier. Recovery orchestration
// must first restore a safe host state; a new process cannot guess ownership.
func (s *Store) EndMaintenance(ctx context.Context, token string) error {
	if token == "" {
		return ErrMaintenanceOwner
	}
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM settings WHERE key=? AND value=?", maintenanceKey, token)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrMaintenanceOwner
		}
		return nil
	})
}

// RequireAdmission runs in the same transaction as the new durable intent.
func RequireAdmission(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key=?", maintenanceKey).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrMaintenance
	}
	return nil
}
