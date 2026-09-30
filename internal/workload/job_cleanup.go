package workload

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func cleanupKey(attempt string) string { return "job.cleanup." + attempt }

// Cleanup intent is journaled before start can reach the privileged supervisor.
// Retry IDs are stable within a policy generation, avoiding duplicate journals.
func (s *Service) releaseJobResources(ctx context.Context, j Job) error {
	suffix := j.AttemptID + "-" + strconv.FormatInt(s.PolicyGeneration, 36)
	key := cleanupKey(j.AttemptID)
	if _, err := s.Store.DB.ExecContext(ctx, "UPDATE settings SET value=json_set(value,'$.lastAttempt',?) WHERE key=?", time.Now().Unix(), key); err != nil {
		return err
	}
	if _, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: "cleanup-stop-" + suffix, Action: "stop", Revision: 2, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
		return err
	}
	if _, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: "cleanup-purge-" + suffix, Action: "purge", Revision: 3, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
		return err
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE settings SET value=json_set(value,'$.state','done') WHERE key=?", key); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE jobs SET error_code=NULL WHERE id=? AND error_code='CLEANUP_PENDING'", j.AttemptID)
		return err
	})
}

func (s *Service) cleanupJobResources(ctx context.Context) error {
	if s.Backend == nil {
		return nil
	}
	var id string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT jobs.id FROM jobs JOIN settings ON settings.key='job.cleanup.'||jobs.id
WHERE jobs.state IN('succeeded','failed','cancelled','interrupted') AND json_extract(CASE WHEN json_valid(settings.value) THEN settings.value ELSE '{}' END,'$.state')='pending'
AND json_type(CASE WHEN json_valid(settings.value) THEN settings.value ELSE '{}' END,'$.lastAttempt')='integer'
AND json_extract(CASE WHEN json_valid(settings.value) THEN settings.value ELSE '{}' END,'$.lastAttempt')>=0
AND json_extract(CASE WHEN json_valid(settings.value) THEN settings.value ELSE '{}' END,'$.lastAttempt')<=? ORDER BY json_extract(CASE WHEN json_valid(settings.value) THEN settings.value ELSE '{}' END,'$.lastAttempt') LIMIT 1`, time.Now().Unix()-30).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	job, err := s.attempt(ctx, id)
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.releaseJobResources(deadline, job)
}
