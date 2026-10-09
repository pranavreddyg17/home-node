package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// ChannelOwnershipIntent binds one directory inode to a reserved domain.
// Persisting it does not authorize chown, socket adoption or domain activation.
type ChannelOwnershipIntent struct {
	InstanceID, ImageSHA256  string
	UID, GuestGID, AccessGID uint32
	Device, Inode            uint64
}

func (m *Manager) channelAccessGID() (uint32, error) {
	var gid int
	switch backend := m.Backend.(type) {
	case LinuxBackend:
		gid = backend.TransferGID
	case *LinuxBackend:
		if backend == nil {
			return 0, ErrPolicy
		}
		gid = backend.TransferGID
	default:
		return 0, ErrPolicy
	}
	if gid <= 0 || gid > 1<<31-1 || uint32(gid) == m.GuestGID {
		return 0, ErrPolicy
	}
	return uint32(gid), nil
}

func (m *Manager) recordChannelOwnershipIntent(ctx context.Context, intent ChannelOwnershipIntent) error {
	return m.checkChannelOwnershipIntent(ctx, intent, true)
}

func (m *Manager) verifyChannelOwnershipIntent(ctx context.Context, intent ChannelOwnershipIntent) error {
	return m.checkChannelOwnershipIntent(ctx, intent, false)
}

func (m *Manager) checkChannelPreparationDomain(ctx context.Context, d Domain) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Store == nil || m.GuestUIDPool == nil {
		return ErrPolicy
	}
	bound := Domain{ID: d.ID, Image: d.Image}
	if err := m.bindDomainGuestIdentity(ctx, &bound, false); err != nil {
		return err
	}
	if bound.GuestUID != d.GuestUID || bound.GuestGID != d.GuestGID || len(d.Image.SHA256) != 64 || strings.Trim(d.Image.SHA256, "0123456789abcdef") != "" {
		return ErrPolicy
	}
	if _, err := m.channelAccessGID(); err != nil {
		return err
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		var phase, desired, image string
		if err := tx.QueryRowContext(ctx, `SELECT state,desired,image_sha256 FROM runtime_instances WHERE id=?`, d.ID).Scan(&phase, &desired, &image); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "preparing" || desired != "running" || image != d.Image.SHA256 {
			return ErrPolicy
		}
		return ctx.Err()
	})
}

// loadChannelOwnershipIntent authenticates a saved record against a domain
// whose identity was independently bound. The result does not admit any path;
// preparation must match a retained directory descriptor to this exact inode.
func (m *Manager) loadChannelOwnershipIntent(ctx context.Context, d Domain) (ChannelOwnershipIntent, error) {
	if err := ctx.Err(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	if m == nil || m.Store == nil || !guestproto.ValidID(d.ID) || d.GuestUID < 65536 || d.GuestGID == 0 {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	intent := ChannelOwnershipIntent{InstanceID: d.ID}
	if err := m.Store.DB.QueryRowContext(ctx, `SELECT image_sha256,uid,guest_gid,access_gid,device,inode FROM runtime_channel_ownership WHERE instance_id=?`, d.ID).Scan(&intent.ImageSHA256, &intent.UID, &intent.GuestGID, &intent.AccessGID, &intent.Device, &intent.Inode); err != nil {
		return ChannelOwnershipIntent{}, errors.Join(ErrPolicy, err)
	}
	if intent.UID != d.GuestUID || intent.GuestGID != d.GuestGID || intent.ImageSHA256 != d.Image.SHA256 {
		return ChannelOwnershipIntent{}, ErrPolicy
	}
	if err := m.verifyChannelOwnershipIntent(ctx, intent); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	if err := ctx.Err(); err != nil {
		return ChannelOwnershipIntent{}, err
	}
	return intent, nil
}

func (m *Manager) checkChannelOwnershipIntent(ctx context.Context, intent ChannelOwnershipIntent, create bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Store == nil || m.GuestUIDPool == nil || m.validateGuestUIDServiceSeparation(*m.GuestUIDPool) != nil || !guestproto.ValidID(intent.InstanceID) || intent.UID < m.GuestUIDPool.First || intent.UID > m.GuestUIDPool.Last || m.GuestUIDPool.Blocked[intent.UID] || intent.GuestGID != m.GuestGID || intent.GuestGID == 0 || intent.GuestGID > 1<<31-1 || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 || len(intent.ImageSHA256) != 64 || strings.Trim(intent.ImageSHA256, "0123456789abcdef") != "" {
		return ErrPolicy
	}
	accessGID, err := m.channelAccessGID()
	if err != nil || accessGID != intent.AccessGID {
		return ErrPolicy
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := m.validateChannelOwnershipInventory(ctx, tx); err != nil {
			return err
		}
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		var uid, gid, first, last uint32
		if err := tx.QueryRowContext(ctx, `SELECT l.uid,g.gid,p.first_uid,p.last_uid FROM runtime_uid_leases l JOIN runtime_guest_groups g ON g.instance_id=l.instance_id JOIN runtime_uid_pool p ON p.singleton=1 WHERE l.instance_id=?`, intent.InstanceID).Scan(&uid, &gid, &first, &last); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if uid != intent.UID || gid != intent.GuestGID || first != m.GuestUIDPool.First || last != m.GuestUIDPool.Last {
			return ErrPolicy
		}
		var phase, desired, digest string
		if err := tx.QueryRowContext(ctx, `SELECT state,desired,image_sha256 FROM runtime_instances WHERE id=?`, intent.InstanceID).Scan(&phase, &desired, &digest); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if phase != "preparing" || desired != "running" || digest != intent.ImageSHA256 {
			return ErrPolicy
		}
		var saved ChannelOwnershipIntent
		saved.InstanceID = intent.InstanceID
		err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,guest_gid,access_gid,device,inode FROM runtime_channel_ownership WHERE instance_id=?`, intent.InstanceID).Scan(&saved.ImageSHA256, &saved.UID, &saved.GuestGID, &saved.AccessGID, &saved.Device, &saved.Inode)
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
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_channel_ownership VALUES(?,?,?,?,?,?,?)`, intent.InstanceID, intent.ImageSHA256, intent.UID, intent.GuestGID, intent.AccessGID, intent.Device, intent.Inode)
		return err
	})
}

// validateChannelOwnershipInventory prevents orphan records or a changed
// independently configured transfer group from authorizing future admission.
func (m *Manager) validateChannelOwnershipInventory(ctx context.Context, tx *sql.Tx) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_ownership`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	accessGID, err := m.channelAccessGID()
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_channel_ownership o LEFT JOIN runtime_uid_leases l ON l.instance_id=o.instance_id LEFT JOIN runtime_guest_groups g ON g.instance_id=o.instance_id LEFT JOIN runtime_instances r ON r.id=o.instance_id WHERE l.instance_id IS NULL OR g.instance_id IS NULL OR r.id IS NULL OR o.uid!=l.uid OR o.guest_gid!=g.gid OR o.access_gid!=? OR o.image_sha256!=r.image_sha256`, accessGID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrPolicy
	}
	return nil
}
