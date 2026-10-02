package state

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

const backupReminderIntervalKey = "backup.reminder.interval-days"

func (s *Store) BackupReminderInterval(ctx context.Context) (int, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", backupReminderIntervalKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 7, nil
	}
	if err != nil {
		return 0, err
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 90 || strconv.Itoa(days) != raw {
		return 0, ErrMaintenance
	}
	return days, nil
}

func (s *Store) SetBackupReminderInterval(ctx context.Context, device string, days int) error {
	if days < 1 || days > 90 {
		return ErrMaintenance
	}
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireAdmission(tx); err != nil {
			return err
		}
		if err := maintenanceDevice(tx, device); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", backupReminderIntervalKey, strconv.Itoa(days))
		return err
	})
}
