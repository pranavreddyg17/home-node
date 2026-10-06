package supervisor

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// bindDomainGuestIdentity preserves the durable identity across launch and
// auditing. Auditing never allocates a replacement identity. A lease cannot be
// downgraded to shared DAC by removing the independently supplied host policy.
func (m *Manager) bindDomainGuestIdentity(ctx context.Context, d *Domain, reserve bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d == nil || d.GuestUID != 0 || d.GuestGID != 0 || !guestproto.ValidID(d.ID) || m.Store == nil {
		return ErrPolicy
	}
	var uid uint32
	err := m.Store.DB.QueryRowContext(ctx, "SELECT uid FROM runtime_uid_leases WHERE instance_id=?", d.ID).Scan(&uid)
	if m.GuestUIDPool == nil {
		if m.GuestGID != 0 || err == nil {
			return ErrPolicy
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	}
	pool := *m.GuestUIDPool
	if pool.validate() != nil || m.GuestGID == 0 || m.GuestGID > 1<<31-1 {
		return ErrPolicy
	}
	if reserve {
		uid, err = m.ReserveGuestUID(ctx, d.ID, pool)
	} else if err == nil && (uid < pool.First || uid > pool.Last || pool.Blocked[uid]) {
		return ErrPolicy
	}
	if err != nil {
		return err
	}
	var first, last uint32
	if err = m.Store.DB.QueryRowContext(ctx, "SELECT first_uid,last_uid FROM runtime_uid_pool WHERE singleton=1").Scan(&first, &last); err != nil {
		return err
	}
	if first != pool.First || last != pool.Last {
		return ErrPolicy
	}
	var gid uint32
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT gid FROM runtime_guest_groups WHERE instance_id=?", d.ID).Scan(&gid)
		if errors.Is(err, sql.ErrNoRows) && reserve {
			gid = m.GuestGID
			_, err = tx.ExecContext(ctx, "INSERT INTO runtime_guest_groups VALUES(?,?)", d.ID, gid)
			return err
		}
		if err != nil {
			return err
		}
		if gid != m.GuestGID {
			return ErrPolicy
		}
		return nil
	})
	if err != nil {
		return err
	}
	d.GuestUID, d.GuestGID = uid, gid
	return nil
}
