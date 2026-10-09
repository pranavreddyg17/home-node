package supervisor

import (
	"context"
	"database/sql"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Caller retains start/runtime exclusion and tears down any failed launch.
// guard retains external preparation authority through runtime publication.
func (m *Manager) launchPreparedDomain(ctx context.Context, r Request, d Domain, guard func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Backend == nil || m.Store == nil || guard == nil || d.ID != r.InstanceID || !guestproto.ValidID(d.ID) || !guestproto.ValidID(r.OperationID) || r.Revision < 1 {
		return ErrPolicy
	}
	check := func() error {
		if err := guard(ctx); err != nil {
			return err
		}
		current, err := m.Inspect(ctx, r.InstanceID)
		if err != nil {
			return err
		}
		if current.Desired != "running" || current.Revision != r.Revision || current.State != "preparing" {
			return ErrPolicy
		}
		return guard(ctx)
	}
	if err := check(); err != nil {
		return err
	}
	if err := m.Backend.Start(ctx, d); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if err := m.verifyPreparedDomain(ctx, d, r.Revision); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE runtime_instances SET state='running' WHERE id=? AND revision=? AND state='preparing' AND desired='running'", r.InstanceID, r.Revision)
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
		result, err = tx.Exec("UPDATE runtime_operations SET state='succeeded' WHERE id=? AND instance_id=? AND state='pending'", r.OperationID, r.InstanceID)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrPolicy
		}
		return nil
	}); err != nil {
		return err
	}
	return guard(ctx)
}
