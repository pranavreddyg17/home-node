package supervisor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const maxVideoRetirementIntentBytes = 64 << 10

type videoRetirementIntent struct {
	Schema    int                     `json:"schema"`
	Request   Request                 `json:"request"`
	Ownership videoRetirementSnapshot `json:"ownership"`
}

type videoRetirementRecord struct {
	Intent videoRetirementIntent
	Stage  string
}

func videoRetirementKey(id string) string { return "runtime.video-retirement." + id }

func videoRetirementStagePending(stage string) bool {
	switch stage {
	case "prepared", "socket-removing", "socket-removed", "volume-removing", "volume-removed", "directory-removing", "directory-removed":
		return true
	default:
		return false
	}
}

// Persist immutable provenance and operation identity atomically, before physical
// effects. The caller must retain runtime exclusion and supply a read-only live
// stopped/process guard. A returned record alone grants no filesystem authority.
func (m *Manager) prepareVideoRetirement(ctx context.Context, r Request, stopped func(context.Context) error) (record videoRetirementRecord, result error) {
	defer func() {
		if result != nil {
			record = videoRetirementRecord{}
		}
	}()
	if m == nil || m.Store == nil || stopped == nil {
		return record, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return record, err
	}
	if err := stopped(ctx); err != nil {
		return record, err
	}
	var canonical []byte
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		snapshot, err := m.loadVideoRetirementSnapshotTx(ctx, tx, r)
		if err != nil {
			return err
		}
		record.Intent = videoRetirementIntent{Schema: 1, Request: r, Ownership: snapshot}
		canonical, err = json.Marshal(record.Intent)
		if err != nil || len(canonical) > maxVideoRetirementIntentBytes {
			return errors.Join(ErrPolicy, err)
		}
		key := videoRetirementKey(r.InstanceID)
		var saved, stage string
		readErr := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=65536 THEN value ELSE NULL END FROM settings WHERE key=?`, key).Scan(&saved)
		stageErr := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=32 THEN value ELSE NULL END FROM settings WHERE key=?`, key+".stage").Scan(&stage)
		fresh := errors.Is(readErr, sql.ErrNoRows)
		if fresh {
			if !errors.Is(stageErr, sql.ErrNoRows) {
				return errors.Join(ErrPolicy, stageErr)
			}
		} else if readErr != nil || stageErr != nil || saved != string(canonical) || !videoRetirementStagePending(stage) {
			return errors.Join(ErrPolicy, readErr, stageErr)
		}
		replay, err := m.operation(tx, r)
		if err != nil {
			return err
		}
		// No interrupted transaction can legitimately publish just one of these.
		if fresh == replay {
			return ErrPolicy
		}
		var instanceID, phase string
		if err := tx.QueryRowContext(ctx, `SELECT instance_id,state FROM runtime_operations WHERE id=?`, r.OperationID).Scan(&instanceID, &phase); err != nil {
			return err
		}
		if instanceID != r.InstanceID || phase != "pending" {
			return ErrPolicy
		}
		if fresh {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?),(?,?)`, key, string(canonical), key+".stage", "prepared"); err != nil {
				return err
			}
			stage = "prepared"
		}
		record.Stage = stage
		return ctx.Err()
	})
	if result != nil {
		return record, result
	}
	if result = stopped(ctx); result != nil {
		return record, result
	}
	observed, err := m.loadVideoRetirementSnapshot(ctx, r)
	if err != nil {
		return record, err
	}
	confirmed, err := json.Marshal(videoRetirementIntent{Schema: 1, Request: r, Ownership: observed})
	if err != nil || !bytes.Equal(confirmed, canonical) {
		return record, errors.Join(ErrPolicy, err)
	}
	return record, ctx.Err()
}
