package workload

import (
	"context"
	"database/sql"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// DrainMaintenance waits for admitted finite work, then journals one persistent
// app shutdown at a time. The normal worker must continue running while draining.
// Cancellation never releases the barrier or forces a VM off. Success covers
// management inventory only; the backup coordinator must still obtain stable
// disk authority from the supervisor before copying any bytes.
func (s *Service) DrainMaintenance(ctx context.Context, token, device string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		inventory, err := s.Store.InspectMaintenance(ctx, token)
		if err != nil {
			return err
		}
		// Uncertain operations cannot drain themselves and need guided repair.
		var uncertain int
		if err := s.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM operations WHERE state='requires-action'").Scan(&uncertain); err != nil {
			return err
		}
		if uncertain != 0 {
			return ErrConflict
		}
		apps, err := s.Apps(ctx)
		if err != nil {
			return err
		}
		for _, app := range apps {
			if app.State != "running" && app.State != "starting" && app.State != "stopping" && app.State != "stopped" {
				return ErrConflict
			}
		}
		if inventory == (state.MaintenanceInventory{}) {
			return nil
		}
		finite := inventory
		finite.Apps = 0
		if finite == (state.MaintenanceInventory{}) {
			for _, app := range apps {
				if app.State == "running" {
					if _, err := s.MaintenanceStop(ctx, token, device, app.Workload); err != nil {
						return err
					}
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// MaintenanceStop journals a cooperative stop after finite work has drained.
// This is an internal coordinator entry point, not a public approval bypass.
// The caller must own an already established maintenance barrier and supply the
// verified owner device. The normal worker rechecks device authority/expiry.
// No disks may be copied based on this admission alone.
func (s *Service) MaintenanceStop(ctx context.Context, token, device, name string) (Operation, error) {
	var op Operation
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := state.RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		var authorized int
		if err := tx.QueryRow(`SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL
 AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')`, device).Scan(&authorized); err != nil {
			return err
		}
		if authorized != 1 {
			return ErrConflict
		}
		// Bind retries to this barrier without storing its bearer token in the
		// operation's externally visible payload or idempotency key.
		key := sum([]byte("maintenance-stop\x00" + token + "\x00" + name))
		var err error
		op, err = s.appActionInTransaction(tx, device, key, name, "stop", func(tx *sql.Tx) error {
			var blockers int
			if err := tx.QueryRow(`SELECT
 (SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*')+
 (SELECT count(*) FROM transfers WHERE state NOT IN('ready','cancelled','expired'))+
 (SELECT count(*) FROM jobs WHERE state NOT IN('succeeded','failed','cancelled','interrupted'))+
 (SELECT count(*) FROM generations WHERE state NOT IN('succeeded','failed','cancelled','interrupted'))+
 (SELECT count(*) FROM operations WHERE state NOT IN('succeeded','failed','cancelled','interrupted') AND NOT(device_id=? AND idempotency_key=?))+
 (SELECT count(*) FROM settings WHERE key GLOB 'job.cleanup.*' AND CASE WHEN json_valid(value) THEN COALESCE(json_extract(value,'$.state'),'') ELSE '' END<>'done')+
 (SELECT count(*) FROM orphan_objects)`, device, key).Scan(&blockers); err != nil {
				return err
			}
			if blockers != 0 {
				return state.ErrMaintenance
			}
			var phase string
			if err := tx.QueryRow("SELECT state FROM apps WHERE workload=?", name).Scan(&phase); err != nil {
				return err
			}
			if phase != "running" {
				return ErrConflict
			}
			return nil
		})
		return err
	})
	if err != nil {
		return Operation{}, err
	}
	return op, nil
}

// MaintenanceRestart restores only an app successfully stopped by this barrier
// for this device. The coordinator must first finish disk copying and release
// supervisor maintenance. It retains management admission until restarts are
// observed; the normal worker rechecks current authority and catalog policy.
func (s *Service) MaintenanceRestart(ctx context.Context, token, device, name string) (Operation, error) {
	var op Operation
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := state.RequireMaintenanceOwner(tx, token); err != nil {
			return err
		}
		var authorized int
		if err := tx.QueryRow(`SELECT count(*) FROM devices WHERE id=? AND revoked_at IS NULL AND EXISTS(SELECT 1 FROM json_each(capabilities) WHERE value='admin')`, device).Scan(&authorized); err != nil {
			return err
		}
		if authorized != 1 {
			return ErrConflict
		}
		stopKey := sum([]byte("maintenance-stop\x00" + token + "\x00" + name))
		var stopID string
		if err := tx.QueryRow("SELECT id FROM operations WHERE device_id=? AND idempotency_key=? AND kind='app.stop' AND state='succeeded'", device, stopKey).Scan(&stopID); err != nil {
			return err
		}
		key := sum([]byte("maintenance-restart\x00" + token + "\x00" + name))
		var err error
		op, err = s.appActionInTransaction(tx, device, key, name, "start", func(tx *sql.Tx) error {
			var owned int
			if err := tx.QueryRow("SELECT count(*) FROM apps WHERE workload=? AND operation_id=? AND state='stopped'", name, stopID).Scan(&owned); err != nil {
				return err
			}
			if owned != 1 {
				return ErrConflict
			}
			var activities int
			if err := tx.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&activities); err != nil {
				return err
			}
			if activities != 0 {
				return state.ErrMaintenance
			}
			return nil
		})
		return err
	})
	if err != nil {
		return Operation{}, err
	}
	return op, nil
}
