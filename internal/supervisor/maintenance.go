package supervisor

import (
	"context"
	"database/sql"

	"github.com/pranavreddyg17/home-node/internal/state"
)

const runtimeMaintenanceKey = "runtime.maintenance"

func requireRuntimeAdmission(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrPolicy
	}
	return nil
}

// BeginRuntimeMaintenance closes supervisor mutation admission after recorded
// runtime work is terminal. Its durable barrier does not expire on a crash.
// This is only the admission component of a disk lease: it does not drain calls
// already outside their admission transaction, attest clean unmount, pin disk
// descriptors, or freeze independent root recovery/audit. It must not be used
// alone to authorize a backup copy. No public controller route exposes it.
func (m *Manager) BeginRuntimeMaintenance(ctx context.Context) (string, error) {
	token := state.Random()
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		var blockers int
		if err := tx.QueryRow(`SELECT
 (SELECT count(*) FROM runtime_instances WHERE state NOT IN('stopped','removed') OR desired<>'stopped')+
 (SELECT count(*) FROM runtime_operations WHERE state NOT IN('succeeded','failed','interrupted'))`).Scan(&blockers); err != nil {
			return err
		}
		if blockers != 0 {
			return ErrPolicy
		}
		_, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", runtimeMaintenanceKey, token)
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// EndRuntimeMaintenance requires the original coordinator token. Higher-level
// recovery must establish safe disk/restart state before calling this method.
func (m *Manager) EndRuntimeMaintenance(ctx context.Context, token string) error {
	if token == "" {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("DELETE FROM settings WHERE key=? AND value=?", runtimeMaintenanceKey, token)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrPolicy
		}
		return nil
	})
}
