package state

import (
	"context"
	"database/sql"
	"math"
)

// RebindRecoveryTx operates only on an exclusively owned disconnected recovery
// database whose schema and authority checks pass. The caller must journal the
// new identity, epoch and disk mappings before effects and roll back on error.
// It creates no passkey/device trust or runtime policy and keeps apps stopped.
// Exact retries are accepted without advancing application revisions again.
func RebindRecoveryTx(ctx context.Context, tx *sql.Tx, ownerID string, epoch int64, instances map[string]string) error {
	if tx == nil || !recoveryInstanceID.MatchString(ownerID) || epoch < 1 || instances == nil || len(instances) > 2 {
		return ErrRecovery
	}
	apps, err := validateRecoveryDatabase(ctx, tx)
	if err != nil {
		return err
	}
	if len(apps) != len(instances) {
		return ErrRecovery
	}
	seen := map[string]bool{}
	for _, app := range apps {
		target, ok := instances[app.Workload]
		if !ok || !recoveryInstanceID.MatchString(target) || seen[target] {
			return ErrRecovery
		}
		seen[target] = true
	}
	var oldOwner string
	var oldEpoch int64
	if err = tx.QueryRowContext(ctx, "SELECT owner_id,epoch FROM identity WHERE singleton=1").Scan(&oldOwner, &oldEpoch); err != nil {
		return ErrRecovery
	}
	if oldOwner == ownerID && oldEpoch == epoch {
		for _, app := range apps {
			if instances[app.Workload] != app.InstanceID {
				return ErrRecovery
			}
		}
		return ctx.Err()
	}
	if oldOwner == ownerID || epoch <= oldEpoch {
		return ErrRecovery
	}
	for _, app := range apps {
		if seen[app.InstanceID] {
			return ErrRecovery
		}
	}
	var invalid int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM apps WHERE revision=?", int64(math.MaxInt64)).Scan(&invalid); err != nil || invalid != 0 {
		return ErrRecovery
	}
	for _, app := range apps {
		result, updateErr := tx.ExecContext(ctx, "UPDATE apps SET instance_id=?,revision=revision+1 WHERE workload=? AND instance_id=? AND state='stopped' AND operation_id IS NULL", instances[app.Workload], app.Workload, app.InstanceID)
		if updateErr != nil {
			return updateErr
		}
		count, countErr := result.RowsAffected()
		if countErr != nil || count != 1 {
			return ErrRecovery
		}
	}
	result, err := tx.ExecContext(ctx, "UPDATE identity SET owner_id=?,epoch=? WHERE singleton=1 AND owner_id=? AND epoch=? AND claimed=0", ownerID, epoch, oldOwner, oldEpoch)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return ErrRecovery
	}
	if _, err = validateRecoveryDatabase(ctx, tx); err != nil {
		return err
	}
	return ctx.Err()
}
