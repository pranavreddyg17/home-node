package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
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

// WithMaintenanceDisk retains exclusive live-manager access and a read-only
// disk descriptor for a trusted internal copier. No public RPC accepts a path
// or callback. The caller must separately qualify filesystem consistency and
// copy into protected staging; this method neither parses nor repairs ext4.
func (m *Manager) WithMaintenanceDisk(ctx context.Context, token, id string, copyDisk func(context.Context, *os.File, Instance) error) (resultErr error) {
	if token == "" || !guestproto.ValidID(id) || copyDisk == nil {
		return ErrPolicy
	}
	unlock, err := m.lockRuntime(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRow("SELECT value FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&owner); err != nil {
			return err
		}
		if owner != token {
			return ErrPolicy
		}
		return nil
	})
	if err != nil {
		return err
	}
	instance, err := m.Inspect(ctx, id)
	if err != nil {
		return err
	}
	if instance.State != "stopped" || instance.Desired != "stopped" || instance.Workload != "files" && instance.Workload != "ai" {
		return ErrPolicy
	}
	running, err := m.Backend.Running(ctx, id)
	if err != nil {
		return err
	}
	if running {
		return ErrPolicy
	}
	file, err := openMaintenanceVolume(ctx, m.Volumes, id, instance.DataBytes)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	return copyDisk(ctx, file, instance)
}

func requireRuntimeAdmission(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key=? OR key='runtime.maintenance-job'", runtimeMaintenanceKey).Scan(&count); err != nil {
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
	return m.beginRuntimeMaintenance(ctx, "")
}

// BeginRuntimeMaintenanceForJob binds acquisition to a durable coordinator ID.
// A retry returns only that job's still-active token; released jobs cannot create
// another barrier. Job identity is private to the trusted maintenance bridge.
func (m *Manager) BeginRuntimeMaintenanceForJob(ctx context.Context, job string) (string, error) {
	if !guestproto.ValidID(job) {
		return "", ErrPolicy
	}
	return m.beginRuntimeMaintenance(ctx, job)
}

func maintenanceJobHash(job string) string { return state.Hash("runtime-maintenance-v1\x00" + job) }

func (m *Manager) beginRuntimeMaintenance(ctx context.Context, job string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	unlock, err := m.lockRuntime(ctx, true)
	if err != nil {
		return "", err
	}
	defer unlock()
	var replayToken string
	if job != "" {
		err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
			var hash, instance, phase string
			err := tx.QueryRow("SELECT request_hash,instance_id,state FROM runtime_operations WHERE id=?", job).Scan(&hash, &instance, &phase)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if hash != maintenanceJobHash(job) || instance != job || phase != "pending" {
				return ErrPolicy
			}
			var owner string
			if err = tx.QueryRow("SELECT value FROM settings WHERE key='runtime.maintenance-job'").Scan(&owner); err != nil {
				return err
			}
			if owner != job {
				return ErrPolicy
			}
			if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", runtimeMaintenanceKey).Scan(&replayToken); err != nil {
				return err
			}
			if !guestproto.ValidID(replayToken) {
				return ErrPolicy
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	// Close rows before external probes: backends must never run while holding
	// the management database's sole connection. The exclusive live-manager
	// guard prevents its runtime calls from racing these observations.
	rows, err := m.Store.DB.QueryContext(ctx, "SELECT id FROM runtime_instances ORDER BY id LIMIT 4097")
	if err != nil {
		return "", err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return "", err
		}
		if !guestproto.ValidID(id) || len(ids) == 4096 {
			rows.Close()
			return "", ErrPolicy
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		running, err := m.Backend.Running(ctx, id)
		if err != nil {
			return "", err
		}
		if running {
			return "", ErrPolicy
		}
	}
	if replayToken != "" {
		err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
			var blockers int
			if err := tx.QueryRow(`SELECT
 (SELECT count(*) FROM runtime_instances WHERE state NOT IN('stopped','removed') OR desired<>'stopped')+
 (SELECT count(*) FROM runtime_operations WHERE state NOT IN('succeeded','failed','interrupted') AND id<>?)`, job).Scan(&blockers); err != nil {
				return err
			}
			if blockers != 0 {
				return ErrPolicy
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		return replayToken, nil
	}
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
		if job != "" {
			if _, err := tx.Exec("INSERT INTO runtime_operations(id,request_hash,instance_id,state) VALUES(?,?,?,'pending')", job, maintenanceJobHash(job), job); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES('runtime.maintenance-job',?)", job); err != nil {
				return err
			}
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
// The most recent release has a durable hashed receipt for lost-response retry;
// replay acknowledges that release without modifying a newer barrier.
func (m *Manager) EndRuntimeMaintenance(ctx context.Context, token string) error {
	unlock, err := m.lockRuntime(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	if !guestproto.ValidID(token) {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		hash := state.Hash(token)
		var receipt string
		err := tx.QueryRow("SELECT value FROM settings WHERE key='runtime.maintenance-release'").Scan(&receipt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && receipt == hash {
			return nil
		}
		var job string
		err = tx.QueryRow("SELECT value FROM settings WHERE key='runtime.maintenance-job'").Scan(&job)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if job != "" {
			if !guestproto.ValidID(job) {
				return ErrPolicy
			}
			result, err := tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=? AND instance_id=? AND request_hash=? AND state='pending'", job, job, maintenanceJobHash(job))
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
			if _, err = tx.Exec("DELETE FROM settings WHERE key='runtime.maintenance-job'"); err != nil {
				return err
			}
		}
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
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES('runtime.maintenance-release',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", hash)
		return err
	})
}
