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
		var reserved int
		if err := m.Store.DB.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM runtime_uid_pool)+(SELECT count(*) FROM runtime_guest_groups WHERE instance_id=?)+(SELECT count(*) FROM runtime_volume_ownership)", d.ID).Scan(&reserved); err != nil {
			return err
		}
		if reserved != 0 {
			return ErrPolicy
		}
		return nil
	}
	pool := *m.GuestUIDPool
	if m.validateGuestUIDServiceSeparation(pool) != nil || m.GuestGID == 0 || m.GuestGID > 1<<31-1 {
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
		var currentUID, currentFirst, currentLast uint32
		if err := tx.QueryRowContext(ctx, `SELECT l.uid,p.first_uid,p.last_uid FROM runtime_uid_leases l JOIN runtime_uid_pool p ON p.singleton=1 WHERE l.instance_id=?`, d.ID).Scan(&currentUID, &currentFirst, &currentLast); err != nil {
			return err
		}
		if currentUID != uid || currentFirst != pool.First || currentLast != pool.Last {
			return ErrPolicy
		}
		var ownershipUID, ownershipGID uint32
		ownershipErr := tx.QueryRowContext(ctx, `SELECT uid,gid FROM runtime_volume_ownership WHERE instance_id=?`, d.ID).Scan(&ownershipUID, &ownershipGID)
		if ownershipErr != nil && !errors.Is(ownershipErr, sql.ErrNoRows) {
			return ownershipErr
		}
		if ownershipErr == nil && (ownershipUID != uid || ownershipGID != m.GuestGID) {
			return ErrPolicy
		}
		err := tx.QueryRowContext(ctx, "SELECT gid FROM runtime_guest_groups WHERE instance_id=?", d.ID).Scan(&gid)
		if errors.Is(err, sql.ErrNoRows) && reserve {
			if ownershipErr == nil {
				return ErrPolicy
			}
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

// validateGuestIdentityPolicy refuses startup policy drift before reconciliation
// can use a shared identity for a previously reserved runtime. Partial UID-only
// reservations remain retryable, but an orphan group assignment is invalid.
func (m *Manager) validateGuestIdentityPolicy(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Store == nil {
		return ErrPolicy
	}
	if m.GuestUIDPool == nil && m.GuestGID != 0 {
		return ErrPolicy
	}
	if m.GuestUIDPool != nil && (m.validateGuestUIDServiceSeparation(*m.GuestUIDPool) != nil || m.GuestGID == 0 || m.GuestGID > 1<<31-1) {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var first, last uint32
		err := tx.QueryRowContext(ctx, "SELECT first_uid,last_uid FROM runtime_uid_pool WHERE singleton=1").Scan(&first, &last)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && (m.GuestUIDPool == nil || first != m.GuestUIDPool.First || last != m.GuestUIDPool.Last) {
			return ErrPolicy
		}
		var conflicts int
		if errors.Is(err, sql.ErrNoRows) {
			if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM runtime_uid_leases)+(SELECT count(*) FROM runtime_guest_groups)+(SELECT count(*) FROM runtime_volume_ownership)").Scan(&conflicts); err != nil {
				return err
			}
			if conflicts != 0 {
				return ErrPolicy
			}
		}
		if m.GuestUIDPool == nil {
			if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM runtime_uid_leases)+(SELECT count(*) FROM runtime_guest_groups)+(SELECT count(*) FROM runtime_volume_ownership)").Scan(&conflicts); err != nil {
				return err
			}
			if conflicts != 0 {
				return ErrPolicy
			}
			return nil
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM runtime_guest_groups g LEFT JOIN runtime_uid_leases u ON u.instance_id=g.instance_id WHERE u.instance_id IS NULL OR g.gid!=?", m.GuestGID).Scan(&conflicts); err != nil {
			return err
		}
		if conflicts != 0 {
			return ErrPolicy
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_volume_ownership o LEFT JOIN runtime_uid_leases l ON l.instance_id=o.instance_id LEFT JOIN runtime_guest_groups g ON g.instance_id=o.instance_id WHERE l.instance_id IS NULL OR g.instance_id IS NULL OR o.uid!=l.uid OR o.gid!=g.gid`).Scan(&conflicts); err != nil {
			return err
		}
		if conflicts != 0 {
			return ErrPolicy
		}
		rows, err := tx.QueryContext(ctx, "SELECT uid FROM runtime_uid_leases")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uid uint32
			if err := rows.Scan(&uid); err != nil {
				return err
			}
			if uid < m.GuestUIDPool.First || uid > m.GuestUIDPool.Last || m.GuestUIDPool.Blocked[uid] {
				return ErrPolicy
			}
		}
		return errors.Join(rows.Err(), rows.Close())
	})
}

func (m *Manager) validateGuestUIDServiceSeparation(pool GuestUIDPool) error {
	if err := pool.validate(); err != nil {
		return err
	}
	for _, uid := range []uint32{m.Policy.ControllerUID, m.Policy.TransferUID} {
		if uid >= pool.First && uid <= pool.Last {
			return ErrPolicy
		}
	}
	return nil
}
