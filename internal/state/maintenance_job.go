package state

import (
	"context"
	"database/sql"
)

const maintenanceJobPrefix = "host.maintenance-job."

type MaintenanceJob struct{ ID, Device, Phase, RootToken string }

func readMaintenanceJob(tx *sql.Tx) (MaintenanceJob, error) {
	var job MaintenanceJob
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.maintenance-job.*'").Scan(&count); err != nil {
		return job, err
	}
	if count != 5 {
		return job, ErrMaintenance
	}
	var version string
	for key, destination := range map[string]*string{"version": &version, "id": &job.ID, "device": &job.Device, "phase": &job.Phase, "root-token": &job.RootToken} {
		if err := tx.QueryRow("SELECT value FROM settings WHERE key=?", maintenanceJobPrefix+key).Scan(destination); err != nil {
			return MaintenanceJob{}, ErrMaintenance
		}
	}
	if (version != "1" && version != "2") || !recoveryInstanceID.MatchString(job.ID) || !recoveryInstanceID.MatchString(job.Device) || job.RootToken != "" && !recoveryInstanceID.MatchString(job.RootToken) {
		return MaintenanceJob{}, ErrMaintenance
	}
	switch job.Phase {
	case "draining", "freezing", "staging", "publishing", "restoring", "requires-action":
	default:
		return MaintenanceJob{}, ErrMaintenance
	}
	if version == "1" && job.Phase == "requires-action" && job.RootToken == "" {
		return MaintenanceJob{}, ErrMaintenance
	}
	if (job.Phase == "draining" || job.Phase == "freezing") && job.RootToken != "" || (job.Phase == "staging" || job.Phase == "publishing") && job.RootToken == "" {
		return MaintenanceJob{}, ErrMaintenance
	}
	return job, nil
}

func maintenanceDevice(tx *sql.Tx, device string) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')", device).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrMaintenanceOwner
	}
	return nil
}

// BeginMaintenanceJob atomically persists admission closure and coordinator
// identity before draining. Root authority is attached only after acquisition.
// These internal records are excluded from recovery exports and public payloads.
func (s *Store) BeginMaintenanceJob(ctx context.Context, device string) (string, MaintenanceJob, error) {
	if !recoveryInstanceID.MatchString(device) {
		return "", MaintenanceJob{}, ErrMaintenanceOwner
	}
	token := Random()
	job := MaintenanceJob{ID: Random(), Device: device, Phase: "draining"}
	err := s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireAdmission(tx); err != nil {
			return err
		}
		var dispatches int
		if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key=?", backupDispatchKey).Scan(&dispatches); err != nil {
			return err
		}
		if dispatches != 0 {
			return ErrMaintenance
		}
		if err := maintenanceDevice(tx, device); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", maintenanceKey, token); err != nil {
			return err
		}
		for key, value := range map[string]string{"version": "2", "id": job.ID, "device": device, "phase": job.Phase, "root-token": ""} {
			if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", maintenanceJobPrefix+key, value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", MaintenanceJob{}, err
	}
	return token, job, nil
}

func (s *Store) InspectMaintenanceJob(ctx context.Context, token string) (MaintenanceJob, error) {
	var job MaintenanceJob
	err := s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		var err error
		job, err = readMaintenanceJob(tx)
		return err
	})
	return job, err
}

func (s *Store) AdvanceMaintenanceJob(ctx context.Context, token, id, from, to string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if job.ID != id || job.Phase != from {
			return ErrMaintenanceOwner
		}
		if err := maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		// Only an acknowledged attachment may leave freezing. Otherwise the
		// coordinator would lose evidence of a potentially acquired root barrier.
		if from == "freezing" {
			return ErrMaintenance
		}
		allowed := to == "requires-action" && from != "requires-action" || to == "restoring" && from != "restoring" || from == "draining" && to == "freezing" || from == "staging" && to == "publishing"
		if !allowed {
			return ErrMaintenance
		}
		if to == "restoring" {
			if err = requireBackupWorkerStopped(tx, id); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("UPDATE settings SET value='2' WHERE key=?", maintenanceJobPrefix+"version"); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE settings SET value=? WHERE key=?", to, maintenanceJobPrefix+"phase")
		return err
	})
}

func (s *Store) AttachMaintenanceRoot(ctx context.Context, token, id, rootToken string) error {
	if !recoveryInstanceID.MatchString(rootToken) {
		return ErrMaintenanceOwner
	}
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if job.ID != id || job.Phase != "freezing" || job.RootToken != "" {
			return ErrMaintenanceOwner
		}
		if err := maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE settings SET value='2' WHERE key=?", maintenanceJobPrefix+"version"); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE settings SET value=? WHERE key=?", rootToken, maintenanceJobPrefix+"root-token"); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE settings SET value='staging' WHERE key=?", maintenanceJobPrefix+"phase")
		return err
	})
}

// ReleaseMaintenanceRoot invokes the trusted supervisor bridge before clearing
// the recorded token. A crash between those steps needs explicit reconciliation;
// this method never guesses that an unacknowledged release succeeded.
func (s *Store) ReleaseMaintenanceRoot(ctx context.Context, token, id string, release func(context.Context, string) error) error {
	if release == nil {
		return ErrMaintenanceOwner
	}
	job, err := s.InspectMaintenanceJob(ctx, token)
	if err != nil {
		return err
	}
	if job.ID != id || job.Phase != "restoring" || job.RootToken == "" {
		return ErrMaintenanceOwner
	}
	if err = s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		current, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if current != job {
			return ErrMaintenanceOwner
		}
		if err = requireBackupWorkerStopped(tx, id); err != nil {
			return err
		}
		return maintenanceDevice(tx, job.Device)
	}); err != nil {
		return err
	}
	if err = release(ctx, job.RootToken); err != nil {
		return err
	}
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		current, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if current != job {
			return ErrMaintenanceOwner
		}
		if err := maintenanceDevice(tx, current.Device); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE settings SET value='2' WHERE key=?", maintenanceJobPrefix+"version"); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE settings SET value='' WHERE key=?", maintenanceJobPrefix+"root-token")
		return err
	})
}

// CompleteMaintenanceJob verifies recorded restoration before atomically
// removing the journal and admission barrier. It cannot release root authority.
func (s *Store) CompleteMaintenanceJob(ctx context.Context, token, id string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		job, err := readMaintenanceJob(tx)
		if err != nil {
			return err
		}
		if job.ID != id || job.Phase != "restoring" || job.RootToken != "" {
			return ErrMaintenanceOwner
		}
		if err = requireBackupWorkerStopped(tx, id); err != nil {
			return err
		}
		if err := maintenanceDevice(tx, job.Device); err != nil {
			return err
		}
		var blockers int
		if err := tx.QueryRow(`SELECT
   (SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*')+
   (SELECT count(*) FROM operations WHERE state NOT IN('succeeded','failed','cancelled','interrupted'))+
   (SELECT count(*) FROM transfers WHERE state NOT IN('ready','cancelled','expired'))+
   (SELECT count(*) FROM jobs WHERE state NOT IN('succeeded','failed','cancelled','interrupted'))+
   (SELECT count(*) FROM generations WHERE state NOT IN('succeeded','failed','cancelled','interrupted'))+
   (SELECT count(*) FROM orphan_objects)+
   (SELECT count(*) FROM settings WHERE key GLOB 'job.cleanup.*' AND CASE WHEN json_valid(value) THEN COALESCE(json_extract(value,'$.state'),'') ELSE '' END<>'done')`).Scan(&blockers); err != nil {
			return err
		}
		if blockers != 0 {
			return ErrMaintenance
		}
		for _, name := range []string{"files", "ai"} {
			stopKey := Hash("maintenance-stop\x00" + token + "\x00" + name)
			var stops int
			if err := tx.QueryRow("SELECT count(*) FROM operations WHERE idempotency_key=? AND kind='app.stop'", stopKey).Scan(&stops); err != nil {
				return err
			}
			if stops == 0 {
				continue
			}
			if stops != 1 {
				return ErrMaintenance
			}
			restartKey := Hash("maintenance-restart\x00" + token + "\x00" + name)
			var restored int
			if err := tx.QueryRow(`SELECT count(*) FROM operations stop JOIN operations restart ON restart.device_id=stop.device_id JOIN apps app ON app.operation_id=restart.id
    WHERE stop.idempotency_key=? AND stop.device_id=? AND stop.kind='app.stop' AND stop.state='succeeded'
    AND restart.idempotency_key=? AND restart.kind='app.start' AND restart.state='succeeded' AND app.workload=? AND app.state='running'`, stopKey, job.Device, restartKey, name).Scan(&restored); err != nil {
				return err
			}
			if restored != 1 {
				return ErrMaintenance
			}
		}
		if _, err = tx.Exec("DELETE FROM settings WHERE key=? AND value=?", backupDispatchKey, "complete:"+id); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM settings WHERE key GLOB 'host.maintenance-job.*'"); err != nil {
			return err
		}
		_, err = tx.Exec("DELETE FROM settings WHERE key=? AND value=?", maintenanceKey, token)
		return err
	})
}
