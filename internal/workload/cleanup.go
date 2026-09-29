package workload

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// Reclaim one expired upload per tick. Keep its reservation until deletion is
// acknowledged: an unavailable guest must not turn disk usage into free quota.
func (s *Service) expireTransfer(ctx context.Context) error {
	var id string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM transfers WHERE state IN('uploading','verifying') AND expires_at<=? ORDER BY expires_at LIMIT 1", time.Now().Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()
	var device, phase string
	var expires int64
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT device_id,state,expires_at FROM transfers WHERE id=?", id).Scan(&device, &phase, &expires); err != nil {
		return err
	}
	if (phase != "uploading" && phase != "verifying") || expires > time.Now().Unix() {
		return nil
	}
	instance, err := s.app(ctx, "files")
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err = s.call(deadline, instance, guestproto.Request{Operation: "delete", ObjectID: id}); err != nil {
		return err
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE transfers SET state='expired' WHERE id=?", id); err != nil {
			return err
		}
		return state.Event(tx, device, "transfer.expired", id, map[string]any{})
	})
}
