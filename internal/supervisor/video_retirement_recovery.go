package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Recover a completed request using its immutable journal, without requiring
// deleted ownership rows or observing resources that a reused UID now owns.
// Pending journals grant no completion and are handled by normal retirement.
func (m *Manager) completedVideoRetirement(ctx context.Context, r Request) (completed bool, result error) {
	defer func() {
		if result != nil {
			completed = false
		}
	}()
	if m == nil || m.Store == nil || r.Version != 1 || r.Action != "purge" || r.Workload != "" || !guestproto.ValidID(r.InstanceID) || !guestproto.ValidID(r.OperationID) || r.Revision < 1 || r.Revision > 1<<53 || r.PolicyGeneration != m.Policy.Generation {
		return false, ErrPolicy
	}
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var phase string
		err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=32 THEN value ELSE NULL END FROM settings WHERE key=?`, videoRetirementKey(r.InstanceID)+".stage").Scan(&phase)
		if errors.Is(err, sql.ErrNoRows) {
			var orphan int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM settings WHERE key=?`, videoRetirementKey(r.InstanceID)).Scan(&orphan); err != nil {
				return err
			}
			if orphan != 0 {
				return ErrPolicy
			}
			return ctx.Err()
		}
		if err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "completed" {
			if !videoRetirementStagePending(phase) {
				return ErrPolicy
			}
			return ctx.Err()
		}
		var saved string
		if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=65536 THEN value ELSE NULL END FROM settings WHERE key=?`, videoRetirementKey(r.InstanceID)).Scan(&saved); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		var intent videoRetirementIntent
		if err := json.Unmarshal([]byte(saved), &intent); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if intent.Request != r {
			return ErrPolicy
		}
		// Verification requires byte-for-byte canonical JSON, rejecting extra fields,
		// duplicate keys, whitespace alterations and trailing JSON as well as drift.
		if err := m.verifyCompletedVideoRetirementTx(ctx, tx, intent); err != nil {
			return err
		}
		completed = true
		return ctx.Err()
	})
	return completed, result
}
