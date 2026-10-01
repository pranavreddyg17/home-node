package state

import (
	"context"
	"database/sql"
	"errors"
)

var ErrBackupPublication = errors.New("backup publication already claimed or ownership invalid")

const backupPublicationKey = "host.backup-publication"

// ClaimBackupPublication records intent before any external repository write.
// Same-job retries always refuse, including after process restart or lost ACK.
// A later uniquely owned job may replace the prior claim; old tokens cannot
// authorize that job. This claim is not proof of repository success.
func (s *Store) ClaimBackupPublication(ctx context.Context, token, id, device string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if job.ID != id || job.Device != device || job.Phase != "publishing" || job.RootToken == "" {
			return ErrBackupPublication
		}
		if err = maintenanceDevice(tx, device); err != nil {
			return err
		}
		inventory, err := readMaintenanceInventory(tx)
		if err != nil {
			return err
		}
		if inventory != (MaintenanceInventory{}) {
			return ErrBackupPublication
		}
		var previous string
		err = tx.QueryRow("SELECT value FROM settings WHERE key=?", backupPublicationKey).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && (!recoveryInstanceID.MatchString(previous) || previous == id) {
			return ErrBackupPublication
		}
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", backupPublicationKey, id)
		return err
	})
}
