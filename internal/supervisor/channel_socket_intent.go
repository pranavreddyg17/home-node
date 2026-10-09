package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

// ChannelSocketIntent binds one observed socket to a recorded directory and
// runtime revision. Recording alone grants no socket adoption or unlink authority.
type ChannelSocketIntent struct {
	Channel       ChannelOwnershipIntent
	Revision      int64
	Device, Inode uint64
}

func (m *Manager) recordChannelSocketIntent(ctx context.Context, intent ChannelSocketIntent) error {
	return m.checkChannelSocketIntent(ctx, intent, true)
}

func (m *Manager) verifyChannelSocketIntent(ctx context.Context, intent ChannelSocketIntent) error {
	return m.checkChannelSocketIntent(ctx, intent, false)
}

// loadActiveChannelSocketIntent is read-only and accepts the current preparing
// or running revision. It cannot create preparation or retirement authority.
func (m *Manager) loadActiveChannelSocketIntent(ctx context.Context, d Domain, revision int64) (ChannelSocketIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelSocketIntent{}, err
	}
	if m == nil || m.Store == nil || revision < 1 {
		return ChannelSocketIntent{}, ErrPolicy
	}
	bound := Domain{ID: d.ID, Image: d.Image}
	if err := m.bindDomainGuestIdentity(ctx, &bound, false); err != nil {
		return ChannelSocketIntent{}, err
	}
	if bound.GuestUID != d.GuestUID || bound.GuestGID != d.GuestGID {
		return ChannelSocketIntent{}, ErrPolicy
	}
	intent := ChannelSocketIntent{Revision: revision, Channel: ChannelOwnershipIntent{InstanceID: d.ID}}
	err := m.Store.DB.QueryRowContext(ctx, `SELECT device,inode,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode FROM runtime_channel_sockets WHERE instance_id=? AND revision=?`, d.ID, revision).Scan(&intent.Device, &intent.Inode, &intent.Channel.ImageSHA256, &intent.Channel.UID, &intent.Channel.GuestGID, &intent.Channel.AccessGID, &intent.Channel.Device, &intent.Channel.Inode)
	if err != nil {
		return ChannelSocketIntent{}, errors.Join(ErrPolicy, err)
	}
	if intent.Channel.ImageSHA256 != d.Image.SHA256 || intent.Channel.UID != d.GuestUID || intent.Channel.GuestGID != d.GuestGID {
		return ChannelSocketIntent{}, ErrPolicy
	}
	if err := m.verifyChannelSocketIntent(ctx, intent); err != nil {
		return ChannelSocketIntent{}, err
	}
	return intent, nil
}

// loadChannelSocketRetirementIntent authenticates the active historical socket
// record against current preparing policy. It permits neither unlink nor
// retirement publication; callers must retain the exact inode and runtime barrier.
func (m *Manager) loadChannelSocketRetirementIntent(ctx context.Context, d Domain) (ChannelSocketIntent, error) {
	intent, _, err := m.loadChannelSocketRetirementRecord(ctx, d, 0, false)
	return intent, err
}

func (m *Manager) loadChannelSocketRetirementRecord(ctx context.Context, d Domain, exactRevision int64, allowRetired bool) (intent ChannelSocketIntent, retired bool, result error) {
	if err := ctx.Err(); err != nil {
		return intent, false, err
	}
	if m == nil || m.Store == nil || exactRevision < 0 || allowRetired && exactRevision < 1 {
		return intent, false, ErrPolicy
	}
	bound := Domain{ID: d.ID, Image: d.Image}
	if err := m.bindDomainGuestIdentity(ctx, &bound, false); err != nil {
		return intent, false, err
	}
	if bound.GuestUID != d.GuestUID || bound.GuestGID != d.GuestGID {
		return intent, false, ErrPolicy
	}
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		var revision int64
		var phase, desired, image string
		if err := tx.QueryRowContext(ctx, `SELECT revision,state,desired,image_sha256 FROM runtime_instances WHERE id=?`, d.ID).Scan(&revision, &phase, &desired, &image); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "preparing" || desired != "running" || image != d.Image.SHA256 {
			return ErrPolicy
		}
		intent.Channel.InstanceID = d.ID
		var completed int
		if err := tx.QueryRowContext(ctx, `SELECT revision,device,inode,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode,retired FROM runtime_channel_sockets WHERE instance_id=? AND (?=0 OR revision=?) AND (? OR retired=0)`, d.ID, exactRevision, exactRevision, allowRetired).Scan(&intent.Revision, &intent.Device, &intent.Inode, &intent.Channel.ImageSHA256, &intent.Channel.UID, &intent.Channel.GuestGID, &intent.Channel.AccessGID, &intent.Channel.Device, &intent.Channel.Inode, &completed); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if intent.Revision > revision || intent.Channel.UID != d.GuestUID || intent.Channel.GuestGID != d.GuestGID || intent.Channel.ImageSHA256 != d.Image.SHA256 {
			return ErrPolicy
		}
		retired = completed == 1
		return ctx.Err()
	})
	if result != nil {
		return ChannelSocketIntent{}, false, result
	}
	return intent, retired, nil
}

func (m *Manager) checkChannelSocketIntent(ctx context.Context, intent ChannelSocketIntent, create bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Store == nil || m.GuestUIDPool == nil || m.validateGuestUIDServiceSeparation(*m.GuestUIDPool) != nil || intent.Channel.UID < m.GuestUIDPool.First || intent.Channel.UID > m.GuestUIDPool.Last || m.GuestUIDPool.Blocked[intent.Channel.UID] || intent.Channel.GuestGID != m.GuestGID || intent.Revision < 1 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		var first, last uint32
		if err := tx.QueryRowContext(ctx, `SELECT first_uid,last_uid FROM runtime_uid_pool WHERE singleton=1`).Scan(&first, &last); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if first != m.GuestUIDPool.First || last != m.GuestUIDPool.Last {
			return ErrPolicy
		}
		var channel ChannelOwnershipIntent
		channel.InstanceID = intent.Channel.InstanceID
		if err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,guest_gid,access_gid,device,inode FROM runtime_channel_ownership WHERE instance_id=?`, channel.InstanceID).Scan(&channel.ImageSHA256, &channel.UID, &channel.GuestGID, &channel.AccessGID, &channel.Device, &channel.Inode); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if channel != intent.Channel {
			return ErrPolicy
		}
		var revision int64
		var phase, desired string
		if err := tx.QueryRowContext(ctx, `SELECT revision,state,desired FROM runtime_instances WHERE id=?`, channel.InstanceID).Scan(&revision, &phase, &desired); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if revision != intent.Revision || desired != "running" || phase != "preparing" && phase != "running" {
			return ErrPolicy
		}
		var device, inode uint64
		var retired, started int
		err := tx.QueryRowContext(ctx, `SELECT device,inode,retired,retirement_started FROM runtime_channel_sockets WHERE instance_id=? AND revision=?`, channel.InstanceID, revision).Scan(&device, &inode, &retired, &started)
		if err == nil {
			if retired != 0 || started != 0 || device != intent.Device || inode != intent.Inode {
				return ErrPolicy
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !create || phase != "preparing" {
			return ErrPolicy
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_sockets WHERE instance_id=? AND retired=0`, channel.InstanceID).Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return ErrPolicy
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_channel_sockets(instance_id,revision,device,inode,retired,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode) VALUES(?,?,?,?,0,?,?,?,?,?,?)`, channel.InstanceID, revision, intent.Device, intent.Inode, channel.ImageSHA256, channel.UID, channel.GuestGID, channel.AccessGID, channel.Device, channel.Inode)
		return err
	})
}

func validateChannelSocketInventory(ctx context.Context, tx *sql.Tx) error {
	var invalid int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_sockets`).Scan(&invalid); err != nil {
		return err
	}
	if invalid == 0 {
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_sockets s LEFT JOIN runtime_channel_ownership c ON c.instance_id=s.instance_id LEFT JOIN runtime_instances r ON r.id=s.instance_id WHERE c.instance_id IS NULL OR r.id IS NULL OR s.revision<1 OR s.revision>r.revision OR s.device<0 OR s.inode<1 OR s.retired NOT IN (0,1) OR s.retirement_started NOT IN (0,1) OR s.retired>s.retirement_started OR s.image_sha256!=c.image_sha256 OR s.uid!=c.uid OR s.guest_gid!=c.guest_gid OR s.access_gid!=c.access_gid OR s.parent_device!=c.device OR s.parent_inode!=c.inode`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return ErrPolicy
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT instance_id FROM runtime_channel_sockets WHERE retired=0 GROUP BY instance_id HAVING count(*)>1)`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return ErrPolicy
	}
	return nil
}
