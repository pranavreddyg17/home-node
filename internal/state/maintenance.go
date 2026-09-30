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
		var active int
		if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return ErrMaintenance
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

const activityPrefix = "host.activity."

// BeginActivity records a bounded host operation before external side effects.
// A maintenance coordinator must wait for these records to drain. They do not
// expire automatically: interrupted activity requires explicit reconciliation.
func (s *Store) BeginActivity(ctx context.Context, kind string) (string, error) {
	if kind != "trash-expiry" {
		return "", ErrMaintenanceOwner
	}
	token := Random()
	err := s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireAdmission(tx); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", activityPrefix+token, kind)
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) EndActivity(ctx context.Context, token string) error {
	if token == "" {
		return ErrMaintenanceOwner
	}
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM settings WHERE key=?", activityPrefix+token)
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

// MaintenanceActivitiesDrained covers registered activities only, not guest
// processes, admitted transfers/jobs or VM shutdown. It is not disk-copy proof.
func (s *Store) MaintenanceActivitiesDrained(ctx context.Context, token string) (bool, error) {
	ready := false
	err := s.Transaction(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRow("SELECT value FROM settings WHERE key=?", maintenanceKey).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrMaintenanceOwner
			}
			return err
		}
		if token == "" || owner != token {
			return ErrMaintenanceOwner
		}
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&count); err != nil {
			return err
		}
		ready = count == 0
		return nil
	})
	return ready, err
}
