package identity

import (
	"context"
	"database/sql"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// VerifyBackupSnapshotSession rechecks the controller-retained actor against
// current session, device and identity state. Callers must separately bind this
// actor to the exact pending snapshot request; worker-provided device IDs cannot
// be converted into actors. This check grants no maintenance or restore token.
func (s *Service) VerifyBackupSnapshotSession(ctx context.Context, actor Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Store == nil || !guestproto.ValidID(actor.Device.ID) || !actor.Allows("admin") || !actor.Fresh() {
		return ErrDenied
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := checkActor(tx, actor); err != nil {
			return err
		}
		var claimed int
		if err := tx.QueryRow("SELECT claimed FROM identity WHERE singleton=1 AND epoch=?", actor.Epoch).Scan(&claimed); err != nil || claimed != 1 {
			return ErrDenied
		}
		return nil
	})
}
