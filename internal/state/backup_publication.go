package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var ErrBackupPublication = errors.New("backup publication already claimed or ownership invalid")

const backupPublicationKey = "host.backup-publication"

// ClaimBackupPublication records intent before any external repository write.
// Same-job retries always refuse, including after process restart or lost ACK.
// A later uniquely owned job may replace an acknowledged prior publication;
// an uncertain prior outcome requires reconciliation. Old tokens cannot
// authorize the new job. This claim is not proof of repository success.
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
		if errors.Is(err, sql.ErrNoRows) {
			var outcomes int
			if err = tx.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.backup-outcome.*'").Scan(&outcomes); err != nil {
				return err
			}
			if outcomes != 0 {
				return ErrBackupPublication
			}
		} else if err == nil {
			if !recoveryInstanceID.MatchString(previous) || previous == id {
				return ErrBackupPublication
			}
			outcome, readErr := readBackupOutcome(tx, backupOutcomeKey)
			if readErr != nil || outcome.JobID != previous || outcome.Status != "published" {
				return ErrBackupPublication
			}
		}
		if err = writeBackupOutcome(tx, backupOutcomeKey, BackupOutcome{Version: 1, JobID: id, Status: "unknown", ClaimedAt: time.Now().UnixMilli()}); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", backupPublicationKey, id)
		return err
	})
}

const backupOutcomeKey = "host.backup-outcome.current"
const backupLastSuccessKey = "host.backup-outcome.last-success"

var backupSnapshotID = regexp.MustCompile(`^[a-f0-9]{64}$`)

// BackupOutcome records repository publication only. It does not attest cleanup,
// repository integrity, or an application restore test. Unknown includes a crash
// after claim and before durable acknowledgement, even if the repository wrote.
type BackupOutcome struct {
	Version     int    `json:"version"`
	JobID       string `json:"jobId"`
	Status      string `json:"status"`
	SnapshotID  string `json:"snapshotId"`
	ClaimedAt   int64  `json:"claimedAt"`
	PublishedAt int64  `json:"publishedAt"`
}

func validBackupOutcome(outcome BackupOutcome) bool {
	if outcome.Version != 1 || !recoveryInstanceID.MatchString(outcome.JobID) || outcome.ClaimedAt <= 0 {
		return false
	}
	switch outcome.Status {
	case "unknown":
		return outcome.SnapshotID == "" && outcome.PublishedAt == 0
	case "published":
		return backupSnapshotID.MatchString(outcome.SnapshotID) && outcome.PublishedAt >= outcome.ClaimedAt
	}
	return false
}
func readBackupOutcome(tx *sql.Tx, key string) (BackupOutcome, error) {
	var outcome BackupOutcome
	var raw string
	if err := tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&raw); err != nil {
		return outcome, err
	}
	if len(raw) > 512 || json.Unmarshal([]byte(raw), &outcome) != nil || !validBackupOutcome(outcome) {
		return BackupOutcome{}, ErrBackupPublication
	}
	canonical, err := json.Marshal(outcome)
	if err != nil || !bytes.Equal(canonical, []byte(raw)) {
		return BackupOutcome{}, ErrBackupPublication
	}
	return outcome, nil
}
func writeBackupOutcome(tx *sql.Tx, key string, outcome BackupOutcome) error {
	if !validBackupOutcome(outcome) {
		return ErrBackupPublication
	}
	raw, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, string(raw))
	return err
}

// RecordBackupPublished is an idempotent acknowledgement of a validated full
// repository snapshot ID. It cannot replace a different outcome or grant a new
// publication claim. Caller authentication remains the controller's job.
func (s *Store) RecordBackupPublished(ctx context.Context, token, id, device, snapshotID string) error {
	if !backupSnapshotID.MatchString(snapshotID) {
		return ErrBackupPublication
	}
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
		var claim string
		if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", backupPublicationKey).Scan(&claim); err != nil || claim != id {
			return ErrBackupPublication
		}
		outcome, err := readBackupOutcome(tx, backupOutcomeKey)
		if err != nil || outcome.JobID != id {
			return ErrBackupPublication
		}
		if outcome.Status == "published" {
			if outcome.SnapshotID == snapshotID {
				return nil
			}
			return ErrBackupPublication
		}
		outcome.Status, outcome.SnapshotID, outcome.PublishedAt = "published", snapshotID, time.Now().UnixMilli()
		if outcome.PublishedAt < outcome.ClaimedAt {
			outcome.PublishedAt = outcome.ClaimedAt
		}
		if err = writeBackupOutcome(tx, backupOutcomeKey, outcome); err != nil {
			return err
		}
		return writeBackupOutcome(tx, backupLastSuccessKey, outcome)
	})
}

// InspectBackupOutcomes returns bounded internal records without tokens, paths,
// keys, or repository credentials. Missing records return nil; malformed stored
// records fail closed. Public callers must separately enforce owner access.
func readBackupOutcomes(tx *sql.Tx, current, lastSuccess **BackupOutcome) error {
	for key, target := range map[string]**BackupOutcome{backupOutcomeKey: current, backupLastSuccessKey: lastSuccess} {
		outcome, err := readBackupOutcome(tx, key)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if key == backupLastSuccessKey && outcome.Status != "published" {
			return ErrBackupPublication
		}
		*target = &outcome
	}
	var claim string
	err := tx.QueryRow("SELECT value FROM settings WHERE key=?", backupPublicationKey).Scan(&claim)
	if errors.Is(err, sql.ErrNoRows) {
		if *current != nil {
			return ErrBackupPublication
		}
	} else if err != nil {
		return err
	} else if *current == nil || !recoveryInstanceID.MatchString(claim) || (*current).JobID != claim {
		return ErrBackupPublication
	}
	return nil
}

func (s *Store) InspectBackupOutcomes(ctx context.Context) (current, lastSuccess *BackupOutcome, resultErr error) {
	resultErr = s.Transaction(ctx, func(tx *sql.Tx) error { return readBackupOutcomes(tx, &current, &lastSuccess) })
	if resultErr != nil {
		return nil, nil, resultErr
	}
	return
}
