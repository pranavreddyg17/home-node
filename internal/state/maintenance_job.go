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
	if version != "1" || !recoveryInstanceID.MatchString(job.ID) || !recoveryInstanceID.MatchString(job.Device) || job.RootToken != "" && !recoveryInstanceID.MatchString(job.RootToken) {
		return MaintenanceJob{}, ErrMaintenance
	}
	switch job.Phase {
	case "draining", "freezing", "staging", "publishing", "restoring", "requires-action":
	default:
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
		if err := maintenanceDevice(tx, device); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", maintenanceKey, token); err != nil {
			return err
		}
		for key, value := range map[string]string{"version": "1", "id": job.ID, "device": device, "phase": job.Phase, "root-token": ""} {
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
		allowed := to == "requires-action" && from != "requires-action" || to == "restoring" && from != "restoring" || from == "draining" && to == "freezing" || from == "staging" && to == "publishing"
		if !allowed {
			return ErrMaintenance
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
		if _, err = tx.Exec("UPDATE settings SET value=? WHERE key=?", rootToken, maintenanceJobPrefix+"root-token"); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE settings SET value='staging' WHERE key=?", maintenanceJobPrefix+"phase")
		return err
	})
}
