package supervisor

import (
	"context"
	"database/sql"
	"errors"
)

// beginChannelSocketRetirement checkpoints the exact active record before
// physical effects. The caller retains runtime exclusion; descriptor-qualified
// quarantine and unlink are still required before publishing retired status.
func (m *Manager) beginChannelSocketRetirement(ctx context.Context, d Domain, stopped func(context.Context) error) (ChannelSocketIntent, error) {
	if stopped == nil {
		return ChannelSocketIntent{}, ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return ChannelSocketIntent{}, err
	}
	if err := stopped(ctx); err != nil {
		return ChannelSocketIntent{}, err
	}
	intent, err := m.loadChannelSocketRetirementIntent(ctx, d)
	if err != nil {
		return ChannelSocketIntent{}, err
	}
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		changed, err := tx.ExecContext(ctx, `UPDATE runtime_channel_sockets SET retirement_started=1 WHERE instance_id=? AND revision=? AND device=? AND inode=? AND retired=0 AND EXISTS(SELECT 1 FROM runtime_instances r WHERE r.id=runtime_channel_sockets.instance_id AND r.state='preparing' AND r.desired='running' AND r.revision>=runtime_channel_sockets.revision AND r.image_sha256=?)`, d.ID, intent.Revision, intent.Device, intent.Inode, d.Image.SHA256)
		if err != nil {
			return err
		}
		count, err := changed.RowsAffected()
		if err != nil || count != 1 {
			return errors.Join(ErrPolicy, err)
		}
		return ctx.Err()
	})
	if err != nil {
		return ChannelSocketIntent{}, err
	}
	if err := stopped(ctx); err != nil {
		return ChannelSocketIntent{}, err
	}
	verified, err := m.loadChannelSocketRetirementIntent(ctx, d)
	if err != nil || verified != intent {
		return ChannelSocketIntent{}, errors.Join(ErrPolicy, err)
	}
	return intent, nil
}

// publishChannelSocketRetirement is called only inside the retained physical
// retirement scope after inode-qualified removal, directory sync and absence
// checks. It preserves provenance and never recycles the guest UID.
func (m *Manager) publishChannelSocketRetirement(ctx context.Context, d Domain, intent ChannelSocketIntent) error {
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		changed, err := tx.ExecContext(ctx, `UPDATE runtime_channel_sockets SET retired=1 WHERE instance_id=? AND revision=? AND device=? AND inode=? AND retirement_started=1 AND image_sha256=? AND uid=? AND guest_gid=? AND access_gid=? AND parent_device=? AND parent_inode=? AND EXISTS(SELECT 1 FROM runtime_instances r WHERE r.id=runtime_channel_sockets.instance_id AND r.state='preparing' AND r.desired='running' AND r.revision>=runtime_channel_sockets.revision AND r.image_sha256=?)`, d.ID, intent.Revision, intent.Device, intent.Inode, intent.Channel.ImageSHA256, intent.Channel.UID, intent.Channel.GuestGID, intent.Channel.AccessGID, intent.Channel.Device, intent.Channel.Inode, d.Image.SHA256)
		if err != nil {
			return err
		}
		count, err := changed.RowsAffected()
		if err != nil || count != 1 {
			return errors.Join(ErrPolicy, err)
		}
		return ctx.Err()
	})
}
