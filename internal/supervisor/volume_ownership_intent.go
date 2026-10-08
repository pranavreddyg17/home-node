package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// VolumeOwnershipIntent is immutable provenance for a particular inode. Saving
// it alone grants no ownership mutation or runtime activation authority.
type VolumeOwnershipIntent struct {
	InstanceID, ImageSHA256 string
	UID, GID                uint32
	Device, Inode           uint64
	Size                    int64
}

func (m *Manager) recordVolumeOwnershipIntent(ctx context.Context, intent VolumeOwnershipIntent) error {
	return m.checkVolumeOwnershipIntent(ctx, intent, true)
}

// verifyVolumeOwnershipIntent rechecks saved provenance and current preparing
// policy without creating or repairing records. The caller still needs a
// retained stopped-runtime barrier before changing a file's ownership.
func (m *Manager) verifyVolumeOwnershipIntent(ctx context.Context, intent VolumeOwnershipIntent) error {
	return m.checkVolumeOwnershipIntent(ctx, intent, false)
}

func (m *Manager) checkVolumeOwnershipIntent(ctx context.Context, intent VolumeOwnershipIntent, create bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Store == nil || m.GuestUIDPool == nil || m.validateGuestUIDServiceSeparation(*m.GuestUIDPool) != nil || !guestproto.ValidID(intent.InstanceID) || intent.UID < m.GuestUIDPool.First || intent.UID > m.GuestUIDPool.Last || m.GuestUIDPool.Blocked[intent.UID] || intent.GID != m.GuestGID || intent.GID == 0 || intent.GID > 1<<31-1 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 || intent.Size < 16<<20 || intent.Size > 512<<30 || len(intent.ImageSHA256) != 64 || strings.Trim(intent.ImageSHA256, "0123456789abcdef") != "" {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		var uid, gid, first, last uint32
		if err := tx.QueryRowContext(ctx, `SELECT l.uid,g.gid,p.first_uid,p.last_uid FROM runtime_uid_leases l JOIN runtime_guest_groups g ON g.instance_id=l.instance_id JOIN runtime_uid_pool p ON p.singleton=1 WHERE l.instance_id=?`, intent.InstanceID).Scan(&uid, &gid, &first, &last); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if uid != intent.UID || gid != intent.GID || first != m.GuestUIDPool.First || last != m.GuestUIDPool.Last {
			return ErrPolicy
		}
		var phase, desired, digest string
		var size int64
		if err := tx.QueryRowContext(ctx, `SELECT state,desired,image_sha256,data_bytes FROM runtime_instances WHERE id=?`, intent.InstanceID).Scan(&phase, &desired, &digest, &size); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "preparing" || desired != "running" || digest != intent.ImageSHA256 || size != intent.Size {
			return ErrPolicy
		}
		var saved VolumeOwnershipIntent
		saved.InstanceID = intent.InstanceID
		err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,gid,device,inode,size FROM runtime_volume_ownership WHERE instance_id=?`, intent.InstanceID).Scan(&saved.ImageSHA256, &saved.UID, &saved.GID, &saved.Device, &saved.Inode, &saved.Size)
		if err == nil {
			if saved != intent {
				return ErrPolicy
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !create {
			return ErrPolicy
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_volume_ownership(instance_id,image_sha256,uid,gid,device,inode,size) VALUES(?,?,?,?,?,?,?)`, intent.InstanceID, intent.ImageSHA256, intent.UID, intent.GID, intent.Device, intent.Inode, intent.Size)
		return err
	})
}
