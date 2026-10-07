package supervisor

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// GuestUIDPool is independently provisioned host policy, never recovered guest
// metadata. Blocked UIDs are independently observed host conflicts. Reservation
// alone grants no DAC, file ownership or runtime activation authority.
type GuestUIDPool struct {
	First, Last uint32
	Blocked     map[uint32]bool
}

func (p GuestUIDPool) validate() error {
	if p.First < 65536 || p.Last < p.First || p.Last > 1<<31-1 || uint64(p.Last)-uint64(p.First) >= 65536 {
		return ErrPolicy
	}
	return nil
}

func (m *Manager) initializeGuestUIDLeases(ctx context.Context) error {
	_, err := m.Store.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_uid_leases(instance_id TEXT PRIMARY KEY, uid INTEGER NOT NULL UNIQUE CHECK(uid>=65536 AND uid<=2147483647)); CREATE TABLE IF NOT EXISTS runtime_guest_groups(instance_id TEXT PRIMARY KEY, gid INTEGER NOT NULL CHECK(gid>0 AND gid<=2147483647)); CREATE TABLE IF NOT EXISTS runtime_uid_pool(singleton INTEGER PRIMARY KEY CHECK(singleton=1), first_uid INTEGER NOT NULL, last_uid INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS runtime_volume_ownership(instance_id TEXT PRIMARY KEY, image_sha256 TEXT NOT NULL, uid INTEGER NOT NULL, gid INTEGER NOT NULL, device INTEGER NOT NULL, inode INTEGER NOT NULL, size INTEGER NOT NULL, UNIQUE(device,inode));`)
	return err
}

// ReserveGuestUID durably binds a new host UID to a fresh guest identity. The
// pool is immutable after first use; leases are never silently recycled. The
// caller must verify host eligibility and exclusive pool provisioning first.
func (m *Manager) ReserveGuestUID(ctx context.Context, id string, pool GuestUIDPool) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !guestproto.ValidID(id) || m.validateGuestUIDServiceSeparation(pool) != nil || m.Store == nil {
		return 0, ErrPolicy
	}
	var assigned uint32
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var orphaned int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_volume_ownership o LEFT JOIN runtime_uid_leases l ON l.instance_id=o.instance_id LEFT JOIN runtime_guest_groups g ON g.instance_id=o.instance_id WHERE l.instance_id IS NULL OR g.instance_id IS NULL OR o.uid!=l.uid OR o.gid!=g.gid`).Scan(&orphaned); err != nil {
			return err
		}
		if orphaned != 0 {
			return ErrPolicy
		}
		var first, last uint32
		err := tx.QueryRowContext(ctx, "SELECT first_uid,last_uid FROM runtime_uid_pool WHERE singleton=1").Scan(&first, &last)
		if errors.Is(err, sql.ErrNoRows) {
			var assignedRecords int
			if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM runtime_uid_leases)+(SELECT count(*) FROM runtime_guest_groups)+(SELECT count(*) FROM runtime_volume_ownership)").Scan(&assignedRecords); err != nil {
				return err
			}
			if assignedRecords != 0 {
				return ErrPolicy
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO runtime_uid_pool VALUES(1,?,?)", pool.First, pool.Last); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if first != pool.First || last != pool.Last {
			return ErrPolicy
		}
		err = tx.QueryRowContext(ctx, "SELECT uid FROM runtime_uid_leases WHERE instance_id=?", id).Scan(&assigned)
		if err == nil {
			if assigned < pool.First || assigned > pool.Last || pool.Blocked[assigned] {
				return ErrPolicy
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT uid FROM runtime_uid_leases")
		if err != nil {
			return err
		}
		used := make(map[uint32]bool)
		for rows.Next() {
			var uid uint32
			if err = rows.Scan(&uid); err != nil {
				rows.Close()
				return err
			}
			if uid < pool.First || uid > pool.Last {
				rows.Close()
				return ErrPolicy
			}
			used[uid] = true
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return err
		}
		for uid := pool.First; uid <= pool.Last; uid++ {
			if err = ctx.Err(); err != nil {
				return err
			}
			if used[uid] || pool.Blocked[uid] {
				continue
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO runtime_uid_leases VALUES(?,?)", id, uid); err != nil {
				return err
			}
			assigned = uid
			return nil
		}
		return ErrCapacity
	})
	if err != nil {
		return 0, err
	}
	return assigned, nil
}
