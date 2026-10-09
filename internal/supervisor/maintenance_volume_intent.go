package supervisor

import (
	"context"
	"database/sql"
	"errors"
)

// loadMaintenanceVolumeIntent authenticates existing provenance under a
// caller-owned maintenance barrier. It grants no formatting, ownership change,
// cleanup or filesystem-consistency authority, and never creates a receipt.
func (m *Manager) loadMaintenanceVolumeIntent(ctx context.Context, token string, d Domain, revision int64) (VolumeOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	if m == nil || m.Store == nil || m.GuestUIDPool == nil || token == "" || revision < 1 {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	bound := Domain{ID: d.ID, Image: d.Image}
	if err := m.bindDomainGuestIdentity(ctx, &bound, false); err != nil {
		return VolumeOwnershipIntent{}, err
	}
	if bound.GuestUID != d.GuestUID || bound.GuestGID != d.GuestGID {
		return VolumeOwnershipIntent{}, ErrPolicy
	}
	intent := VolumeOwnershipIntent{InstanceID: d.ID}
	err := m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, runtimeMaintenanceKey).Scan(&owner); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if owner != token {
			return ErrPolicy
		}
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		var phase, desired, digest string
		var size, currentRevision int64
		if err := tx.QueryRowContext(ctx, `SELECT state,desired,image_sha256,data_bytes,revision FROM runtime_instances WHERE id=?`, d.ID).Scan(&phase, &desired, &digest, &size, &currentRevision); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "stopped" || desired != "stopped" || digest != d.Image.SHA256 || size != d.Image.DataBytes || currentRevision != revision {
			return ErrPolicy
		}
		if err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,gid,device,inode,size FROM runtime_volume_ownership WHERE instance_id=?`, d.ID).Scan(&intent.ImageSHA256, &intent.UID, &intent.GID, &intent.Device, &intent.Inode, &intent.Size); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if intent.ImageSHA256 != digest || intent.Size != size || intent.UID != d.GuestUID || intent.GID != d.GuestGID || intent.Inode == 0 {
			return ErrPolicy
		}
		return ctx.Err()
	})
	if err != nil {
		return VolumeOwnershipIntent{}, err
	}
	return intent, nil
}
