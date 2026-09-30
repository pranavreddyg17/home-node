package workload

import (
	"context"
	"database/sql"

	"github.com/pranavreddyg17/home-node/internal/state"
)

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
