package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// A snapshot is immutable journal provenance, not physical retirement authority.
// A caller must separately retain runtime exclusion, verify guest/process exit,
// pin the named filesystem objects and checkpoint each destructive effect.
type videoRetirementSnapshot struct {
	InstanceID     string
	ImageSHA256    string
	SourceRevision int64
	Volume         VolumeOwnershipIntent
	Channel        ChannelOwnershipIntent
	Sockets        []videoRetirementSocket
}

type videoRetirementSocket struct {
	Revision                   int64
	Device, Inode              uint64
	Retired, RetirementStarted int
}

func (m *Manager) loadVideoRetirementSnapshot(ctx context.Context, r Request) (snapshot videoRetirementSnapshot, result error) {
	defer func() {
		if result != nil {
			snapshot = videoRetirementSnapshot{}
		}
	}()
	if m == nil || m.Store == nil || m.GuestUIDPool == nil || r.Version != 1 || r.Action != "purge" || r.Workload != "" || !guestproto.ValidID(r.InstanceID) || !guestproto.ValidID(r.OperationID) || r.PolicyGeneration != m.Policy.Generation || r.Revision < 1 || r.Revision > 1<<53 || m.validateGuestUIDServiceSeparation(*m.GuestUIDPool) != nil {
		return snapshot, ErrPolicy
	}
	accessGID, err := m.channelAccessGID()
	if err != nil {
		return snapshot, err
	}
	result = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := requireRuntimeAdmission(tx); err != nil {
			return err
		}
		var workload, phase, desired string
		var uid, gid, first, last uint32
		var size int64
		snapshot.InstanceID = r.InstanceID
		err := tx.QueryRowContext(ctx, `SELECT r.workload,r.state,r.desired,r.image_sha256,r.data_bytes,r.revision,l.uid,g.gid,p.first_uid,p.last_uid FROM runtime_instances r JOIN runtime_uid_leases l ON l.instance_id=r.id JOIN runtime_guest_groups g ON g.instance_id=r.id JOIN runtime_uid_pool p ON p.singleton=1 WHERE r.id=?`, r.InstanceID).Scan(&workload, &phase, &desired, &snapshot.ImageSHA256, &size, &snapshot.SourceRevision, &uid, &gid, &first, &last)
		if err != nil {
			return errors.Join(ErrPolicy, err)
		}
		pool := m.GuestUIDPool
		if workload != "video" || phase != "stopped" || desired != "stopped" || snapshot.SourceRevision < 1 || r.Revision <= snapshot.SourceRevision || first != pool.First || last != pool.Last || uid < first || uid > last || pool.Blocked[uid] || gid != m.GuestGID || gid == 0 || gid == accessGID || size < 16<<20 || size > 512<<30 || len(snapshot.ImageSHA256) != 64 || strings.Trim(snapshot.ImageSHA256, "0123456789abcdef") != "" {
			return ErrPolicy
		}
		v := &snapshot.Volume
		v.InstanceID = r.InstanceID
		if err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,gid,device,inode,size FROM runtime_volume_ownership WHERE instance_id=?`, r.InstanceID).Scan(&v.ImageSHA256, &v.UID, &v.GID, &v.Device, &v.Inode, &v.Size); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if v.ImageSHA256 != snapshot.ImageSHA256 || v.UID != uid || v.GID != gid || v.Size != size || v.Device > math.MaxInt64 || v.Inode == 0 || v.Inode > math.MaxInt64 {
			return ErrPolicy
		}
		c := &snapshot.Channel
		c.InstanceID = r.InstanceID
		if err := tx.QueryRowContext(ctx, `SELECT image_sha256,uid,guest_gid,access_gid,device,inode FROM runtime_channel_ownership WHERE instance_id=?`, r.InstanceID).Scan(&c.ImageSHA256, &c.UID, &c.GuestGID, &c.AccessGID, &c.Device, &c.Inode); err != nil {
			return errors.Join(ErrPolicy, err)
		}
		if c.ImageSHA256 != snapshot.ImageSHA256 || c.UID != uid || c.GuestGID != gid || c.AccessGID != accessGID || c.Device > math.MaxInt64 || c.Inode == 0 || c.Inode > math.MaxInt64 {
			return ErrPolicy
		}
		rows, err := tx.QueryContext(ctx, `SELECT revision,device,inode,retired,retirement_started,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode FROM runtime_channel_sockets WHERE instance_id=? ORDER BY revision LIMIT 257`, r.InstanceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		active := 0
		for rows.Next() {
			var socket videoRetirementSocket
			var digest string
			var suid, guestGID, transferGID uint32
			var parentDevice, parentInode uint64
			if err := rows.Scan(&socket.Revision, &socket.Device, &socket.Inode, &socket.Retired, &socket.RetirementStarted, &digest, &suid, &guestGID, &transferGID, &parentDevice, &parentInode); err != nil {
				return err
			}
			if len(snapshot.Sockets) == 256 || socket.Revision < 1 || socket.Revision > snapshot.SourceRevision || socket.Device > math.MaxInt64 || socket.Inode == 0 || socket.Inode > math.MaxInt64 || socket.Retired < 0 || socket.Retired > 1 || socket.RetirementStarted < 0 || socket.RetirementStarted > 1 || socket.Retired > socket.RetirementStarted || digest != snapshot.ImageSHA256 || suid != uid || guestGID != gid || transferGID != accessGID || parentDevice != c.Device || parentInode != c.Inode {
				return ErrPolicy
			}
			if socket.Retired == 0 {
				active++
			}
			if active > 1 {
				return ErrPolicy
			}
			snapshot.Sockets = append(snapshot.Sockets, socket)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(snapshot.Sockets) == 0 {
			return ErrPolicy
		}
		return ctx.Err()
	})
	return snapshot, result
}
