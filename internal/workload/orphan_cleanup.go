package workload

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// The copy and metadata commit share this lock. A failed copy remains charged
// at its maximum possible size until the files guest acknowledges deletion.
func (s *Service) cleanupOrphanObject(ctx context.Context) error {
	var id string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM orphan_objects WHERE workload='files' ORDER BY created_at LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock := s.lock(id)
	defer unlock()
	var exists bool
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM orphan_objects WHERE id=?)", id).Scan(&exists); err != nil || !exists {
		return err
	}
	var published bool
	if err = s.Store.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE id=?)", id).Scan(&published); err != nil {
		return err
	}
	if published {
		return ErrConflict
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
	_, err = s.Store.DB.ExecContext(ctx, "DELETE FROM orphan_objects WHERE id=?", id)
	return err
}
