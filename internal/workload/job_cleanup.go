package workload

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func cleanupKey(attempt string) string { return "job.cleanup." + attempt }

// Cleanup intent is journaled before start can reach the privileged supervisor.
// Retry IDs are stable within a policy generation, avoiding duplicate journals.
func (s *Service) releaseJobResources(ctx context.Context, j Job) error {
	suffix := j.AttemptID + "-" + strconv.FormatInt(s.PolicyGeneration, 36)
	key := cleanupKey(j.AttemptID)
	var payload string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&payload); err != nil {
		return err
	}
	journal, err := parseJobCleanup(payload)
	if err != nil {
		_, persistErr := s.Store.DB.ExecContext(ctx, "UPDATE settings SET value=json_object('state','requires-action','original',value) WHERE key=? AND value=?", key, payload)
		return errors.Join(err, persistErr)
	}
	if journal.State == "done" {
		return nil
	}
	journal.LastAttempt = time.Now().Unix()
	encoded, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	result, err := s.Store.DB.ExecContext(ctx, "UPDATE settings SET value=? WHERE key=? AND value=?", string(encoded), key, payload)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	if _, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: "cleanup-stop-" + suffix, Action: "stop", Revision: 2, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
		return err
	}
	if _, err := s.Backend.Apply(ctx, supervisor.Request{Version: 1, OperationID: "cleanup-purge-" + suffix, Action: "purge", Revision: 3, InstanceID: j.InstanceID, PolicyGeneration: s.PolicyGeneration}); err != nil {
		return err
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE settings SET value=json_set(value,'$.state','done') WHERE key=? AND value=?", key, string(encoded))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
		_, err = tx.Exec("UPDATE jobs SET error_code=NULL WHERE id=? AND error_code='CLEANUP_PENDING'", j.AttemptID)
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

type jobCleanupJournal struct {
	State       string `json:"state"`
	LastAttempt int64  `json:"lastAttempt"`
}

func parseJobCleanup(payload string) (jobCleanupJournal, error) {
	var journal jobCleanupJournal
	if len(payload) > 256 || !utf8.ValidString(payload) {
		return journal, ErrConflict
	}
	d := json.NewDecoder(strings.NewReader(payload))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return journal, ErrConflict
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] || (key != "state" && key != "lastAttempt") {
			return journal, ErrConflict
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || string(raw) == "null" {
			return journal, ErrConflict
		}
		if key == "state" {
			err = json.Unmarshal(raw, &journal.State)
		} else {
			err = json.Unmarshal(raw, &journal.LastAttempt)
		}
		if err != nil {
			return journal, ErrConflict
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		return journal, ErrConflict
	}
	if _, err = d.Token(); err != io.EOF || len(seen) != 2 || (journal.State != "pending" && journal.State != "done") || journal.LastAttempt < 0 {
		return journal, ErrConflict
	}
	return journal, nil
}
