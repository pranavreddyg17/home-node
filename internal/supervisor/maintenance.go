package supervisor

import (
	"context"
	"database/sql"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

const runtimeMaintenanceKey = "runtime.maintenance"

// The service owns one live Manager. Readers cover whole external calls, not
// just database admission. Maintenance waits with its caller's deadline rather
// than blocking indefinitely on a mutex. Audit/recovery remain concurrent with
// normal mutations, preserving emergency teardown while a guest is stopping.
func (m *Manager) lockRuntime(ctx context.Context, exclusive bool) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		acquired := false
		unlock := m.runtimeMu.RUnlock
		if exclusive {
			acquired = m.runtimeMu.TryLock()
			unlock = m.runtimeMu.Unlock
		} else {
			acquired = m.runtimeMu.TryRLock()
		}
		if acquired {
			if err := ctx.Err(); err != nil {
				unlock()
				return nil, err
			}
			return unlock, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

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
// It waits for admitted calls, audit and recovery on this live Manager before
// checking inventory. This is only part of a disk lease: it does not attest
// clean unmount, pin disk descriptors or freeze other root processes. It must not be used
// alone to authorize a backup copy. No public controller route exposes it.
func (m *Manager) BeginRuntimeMaintenance(ctx context.Context) (string, error) {
	unlock, err := m.lockRuntime(ctx, true)
	if err != nil {
		return "", err
	}
	defer unlock()
	token := state.Random()
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
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
	unlock, err := m.lockRuntime(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
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
