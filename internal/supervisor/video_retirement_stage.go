package supervisor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func nextVideoRetirementStage(stage string) string {
	switch stage {
	case "prepared":
		return "socket-removing"
	case "socket-removing":
		return "socket-removed"
	case "socket-removed":
		return "volume-removing"
	case "volume-removing":
		return "volume-removed"
	case "volume-removed":
		return "directory-removing"
	case "directory-removing":
		return "directory-removed"
	default:
		return ""
	}
}

// Authenticate a journal without creating an intent or repairing an operation.
func (m *Manager) readVideoRetirementRecordTx(ctx context.Context, tx *sql.Tx, intent videoRetirementIntent) (stage string, result error) {
	defer func() {
		if result != nil {
			stage = ""
		}
	}()
	if intent.Schema != 1 || len(intent.Ownership.Sockets) > 256 {
		return "", ErrPolicy
	}
	snapshot, err := m.loadVideoRetirementSnapshotTx(ctx, tx, intent.Request)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(videoRetirementIntent{Schema: 1, Request: intent.Request, Ownership: snapshot})
	if err != nil || len(canonical) > maxVideoRetirementIntentBytes {
		return "", errors.Join(ErrPolicy, err)
	}
	supplied, err := json.Marshal(intent)
	if err != nil || !bytes.Equal(supplied, canonical) {
		return "", errors.Join(ErrPolicy, err)
	}
	key := videoRetirementKey(intent.Request.InstanceID)
	var saved, hash, instanceID, phase string
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=65536 THEN value ELSE NULL END FROM settings WHERE key=?`, key).Scan(&saved); err != nil {
		return "", errors.Join(ErrPolicy, err)
	}
	if saved != string(canonical) {
		return "", ErrPolicy
	}
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=32 THEN value ELSE NULL END FROM settings WHERE key=?`, key+".stage").Scan(&stage); err != nil {
		return "", errors.Join(ErrPolicy, err)
	}
	if !videoRetirementStagePending(stage) {
		return "", ErrPolicy
	}
	if err := tx.QueryRowContext(ctx, `SELECT request_hash,instance_id,state FROM runtime_operations WHERE id=?`, intent.Request.OperationID).Scan(&hash, &instanceID, &phase); err != nil {
		return "", errors.Join(ErrPolicy, err)
	}
	request, err := json.Marshal(intent.Request)
	if err != nil || hash != state.Hash(string(request)) || instanceID != intent.Request.InstanceID || phase != "pending" {
		return "", errors.Join(ErrPolicy, err)
	}
	return stage, ctx.Err()
}

// A checkpoint does not establish physical absence or release ownership. Its
// caller must retain the descriptor-qualified scope and supply its live guard.
// Completion and UID release require a separate final transaction.
func (m *Manager) advanceVideoRetirement(ctx context.Context, previous videoRetirementRecord, target string, guard func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || guard == nil || target == "" || nextVideoRetirementStage(previous.Stage) != target {
		return record, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return record, err
	}
	if err := guard(ctx); err != nil {
		return record, err
	}
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		stage, err := m.readVideoRetirementRecordTx(ctx, tx, previous.Intent)
		if err != nil {
			return err
		}
		if stage == target {
			return nil
		} // Exact retry, never a skip past this edge.
		if stage != previous.Stage {
			return ErrPolicy
		}
		changed, err := tx.ExecContext(ctx, `UPDATE settings SET value=? WHERE key=? AND value=?`, target, videoRetirementKey(previous.Intent.Request.InstanceID)+".stage", previous.Stage)
		if err != nil {
			return err
		}
		count, err := changed.RowsAffected()
		if err != nil || count != 1 {
			return errors.Join(ErrPolicy, err)
		}
		return ctx.Err()
	})
	if result != nil {
		return record, result
	}
	if result = guard(ctx); result != nil {
		return record, result
	}
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		stage, err := m.readVideoRetirementRecordTx(ctx, tx, previous.Intent)
		if err != nil || stage != target {
			return errors.Join(ErrPolicy, err)
		}
		record = videoRetirementRecord{Intent: previous.Intent, Stage: target}
		return ctx.Err()
	})
	return record, result
}
