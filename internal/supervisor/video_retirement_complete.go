package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// Commit only after all descriptor scopes have closed under retained start and
// runtime exclusion, and the caller has qualified process exit, complete resource
// absence and safe UID reuse. The callback is trusted internal authority, never
// an RPC parameter. This transaction alone grants no physical cleanup authority.
func (m *Manager) commitVideoRetirement(ctx context.Context, previous videoRetirementRecord, qualifiedAbsence func(context.Context) error) (result error) {
	if m == nil || m.Store == nil || qualifiedAbsence == nil || previous.Stage != "directory-removed" {
		return ErrPolicy
	}
	// A completed replay authenticates its immutable receipt, not a now-reused UID.
	completed := false
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var phase string
		err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=32 THEN value ELSE NULL END FROM settings WHERE key=?`, videoRetirementKey(previous.Intent.Request.InstanceID)+".stage").Scan(&phase)
		if err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "completed" {
			return nil
		}
		if err := m.verifyCompletedVideoRetirementTx(ctx, tx, previous.Intent); err != nil {
			return err
		}
		completed = true
		return nil
	}); err != nil {
		return err
	}
	if completed {
		return nil
	}
	if err := qualifiedAbsence(ctx); err != nil {
		return err
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		phase, err := m.readVideoRetirementRecordTx(ctx, tx, previous.Intent)
		if err != nil {
			return err
		}
		if phase != "directory-removed" {
			return ErrPolicy
		}
		id := previous.Intent.Request.InstanceID
		for _, deletion := range []struct {
			query string
			count int64
		}{
			{`DELETE FROM runtime_channel_sockets WHERE instance_id=?`, int64(len(previous.Intent.Ownership.Sockets))},
			{`DELETE FROM runtime_channel_ownership WHERE instance_id=?`, 1},
			{`DELETE FROM runtime_volume_ownership WHERE instance_id=?`, 1},
			{`DELETE FROM runtime_guest_groups WHERE instance_id=?`, 1},
			{`DELETE FROM runtime_uid_leases WHERE instance_id=?`, 1},
		} {
			change, err := tx.ExecContext(ctx, deletion.query, id)
			if err != nil {
				return err
			}
			n, err := change.RowsAffected()
			if err != nil || n != deletion.count {
				return errors.Join(ErrPolicy, err)
			}
		}
		for _, update := range []struct {
			query string
			args  []any
		}{
			{`UPDATE runtime_instances SET state='removed',revision=? WHERE id=? AND state='stopped' AND desired='stopped' AND revision=?`, []any{previous.Intent.Request.Revision, id, previous.Intent.Ownership.SourceRevision}},
			{`UPDATE runtime_operations SET state='succeeded' WHERE id=? AND instance_id=? AND state='pending'`, []any{previous.Intent.Request.OperationID, id}},
			{`UPDATE settings SET value='completed' WHERE key=? AND value='directory-removed'`, []any{videoRetirementKey(id) + ".stage"}},
		} {
			change, err := tx.ExecContext(ctx, update.query, update.args...)
			if err != nil {
				return err
			}
			n, err := change.RowsAffected()
			if err != nil || n != 1 {
				return errors.Join(ErrPolicy, err)
			}
		}
		if err := m.verifyCompletedVideoRetirementTx(ctx, tx, previous.Intent); err != nil {
			return err
		}
		return ctx.Err()
	})
}

func (m *Manager) verifyCompletedVideoRetirementTx(ctx context.Context, tx *sql.Tx, intent videoRetirementIntent) error {
	if intent.Schema != 1 || intent.Request.Version != 1 || intent.Request.Action != "purge" || intent.Request.Workload != "" || intent.Request.InstanceID != intent.Ownership.InstanceID {
		return ErrPolicy
	}
	canonical, err := json.Marshal(intent)
	if err != nil || len(canonical) > maxVideoRetirementIntentBytes {
		return errors.Join(ErrPolicy, err)
	}
	var saved, phase, hash, operationInstance, operationPhase, runtimePhase, desired, workload, image string
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=65536 THEN value ELSE NULL END FROM settings WHERE key=?`, videoRetirementKey(intent.Request.InstanceID)).Scan(&saved); err != nil {
		return errors.Join(ErrPolicy, err)
	}
	if saved != string(canonical) {
		return ErrPolicy
	}
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=32 THEN value ELSE NULL END FROM settings WHERE key=?`, videoRetirementKey(intent.Request.InstanceID)+".stage").Scan(&phase); err != nil || phase != "completed" {
		return errors.Join(ErrPolicy, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT request_hash,instance_id,state FROM runtime_operations WHERE id=?`, intent.Request.OperationID).Scan(&hash, &operationInstance, &operationPhase); err != nil {
		return errors.Join(ErrPolicy, err)
	}
	request, err := json.Marshal(intent.Request)
	if err != nil || hash != state.Hash(string(request)) || operationInstance != intent.Request.InstanceID || operationPhase != "succeeded" {
		return errors.Join(ErrPolicy, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT state,desired,workload,image_sha256,revision FROM runtime_instances WHERE id=?`, intent.Request.InstanceID).Scan(&runtimePhase, &desired, &workload, &image, &revision); err != nil {
		return errors.Join(ErrPolicy, err)
	}
	if runtimePhase != "removed" || desired != "stopped" || workload != "video" || image != intent.Ownership.ImageSHA256 || revision != intent.Request.Revision {
		return ErrPolicy
	}
	var residual int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?)+(SELECT count(*) FROM runtime_guest_groups WHERE instance_id=?)+(SELECT count(*) FROM runtime_volume_ownership WHERE instance_id=?)+(SELECT count(*) FROM runtime_channel_ownership WHERE instance_id=?)+(SELECT count(*) FROM runtime_channel_sockets WHERE instance_id=?)`, intent.Request.InstanceID, intent.Request.InstanceID, intent.Request.InstanceID, intent.Request.InstanceID, intent.Request.InstanceID).Scan(&residual); err != nil {
		return err
	}
	if residual != 0 {
		return ErrPolicy
	}
	return ctx.Err()
}
