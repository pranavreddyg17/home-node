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
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM transfers WHERE state='cancelling' OR (state IN('uploading','verifying') AND expires_at<=?) ORDER BY expires_at LIMIT 1", time.Now().Unix()).Scan(&id)
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
	if phase != "cancelling" && ((phase != "uploading" && phase != "verifying") || expires > time.Now().Unix()) {
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
	terminal := "expired"
	if phase == "cancelling" {
		terminal = "cancelled"
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE transfers SET state=? WHERE id=?", terminal, id); err != nil {
			return err
		}
		return state.Event(tx, device, "transfer."+terminal, id, map[string]any{})
	})
}

// Persist cancellation before attempting guest cleanup, so loss of a connection
// or an offline Files app cannot resume a transfer the owner has discarded.
func (s *Service) CancelTransfer(ctx context.Context, device, id string) (Transfer, error) {
	unlock := s.lock(id)
	defer unlock()
	t, err := s.Transfer(ctx, device, id)
	if err != nil {
		return t, err
	}
	if t.State == "cancelled" || t.State == "cancelling" || t.State == "expired" {
		return t, nil
	}
	if t.State != "uploading" && t.State != "verifying" {
		return t, ErrConflict
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE transfers SET state='cancelling' WHERE id=?", id); err != nil {
			return err
		}
		return state.Event(tx, device, "transfer.cancelling", id, map[string]any{})
	})
	if err == nil {
		t.State = "cancelling"
	}
	return t, err
}

// expireTrash retains a minimal tombstone because job history and completed
// transfers reference file IDs. Bytes remain charged until durable deletion.
// Negative trash_until denotes an irreversible purge, never a restorable file.
func (s *Service) expireTrash(ctx context.Context) error {
	var id string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM files WHERE trash_until>=0 AND trash_until<=?
AND NOT EXISTS(SELECT 1 FROM jobs WHERE input_id=files.id AND state IN('queued','preparing','running','finalizing','cancelling'))
ORDER BY trash_until LIMIT 1`, time.Now().Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()
	var expiration sql.NullInt64
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT trash_until FROM files WHERE id=?", id).Scan(&expiration); err != nil {
		return err
	}
	if !expiration.Valid || expiration.Int64 < 0 || expiration.Int64 > time.Now().Unix() {
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
		if _, err := tx.Exec("UPDATE files SET trash_until=-1,size=0,name='Removed file',sha256=? WHERE id=?", sum(nil), id); err != nil {
			return err
		}
		return state.Event(tx, "system", "file.expired", id, map[string]any{})
	})
}
