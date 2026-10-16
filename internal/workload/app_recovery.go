package workload

import (
	"database/sql"
	"encoding/json"
	"time"
)

// appRecoveryChain validates all links before any settlement. Strictly falling
// revisions prevent cycles; the bound keeps malformed histories from occupying
// the controller indefinitely. Admission checks the same chain before dispatch.
func appRecoveryChain(tx *sql.Tx, id, workload, instance string, before int64) ([]string, error) {
	var ids []string
	for id != "" {
		if len(ids) >= 256 {
			return nil, ErrConflict
		}
		var kind, phase, payload string
		if err := tx.QueryRow("SELECT kind,state,result FROM operations WHERE id=?", id).Scan(&kind, &phase, &payload); err != nil {
			return nil, err
		}
		if phase != "requires-action" && phase != "interrupted" {
			return nil, ErrConflict
		}
		var wrapper struct {
			Intent json.RawMessage `json:"intent"`
		}
		if err := json.Unmarshal([]byte(payload), &wrapper); err != nil {
			return nil, ErrConflict
		}
		if len(wrapper.Intent) != 0 {
			payload = string(wrapper.Intent)
		}
		intent, err := parseAppIntent(payload, kind)
		if err != nil || intent.Workload != workload || intent.InstanceID != instance || intent.Revision >= before {
			return nil, ErrConflict
		}
		ids = append(ids, id)
		id, before = intent.PreviousOperationID, intent.Revision
	}
	return ids, nil
}

func settleAppRecovery(tx *sql.Tx, id, workload, instance string, before int64) error {
	ids, err := appRecoveryChain(tx, id, workload, instance, before)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec("UPDATE operations SET state='interrupted',updated_at=? WHERE id=? AND state='requires-action'", time.Now().Unix(), id); err != nil {
			return err
		}
	}
	return nil
}
