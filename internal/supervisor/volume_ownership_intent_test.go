package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestVolumeOwnershipIntentBindsLeaseAndInode(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200002}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	ctx := context.Background()
	makeIntent := func() VolumeOwnershipIntent {
		d := Domain{ID: state.Random()}
		if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
			t.Fatal(err)
		}
		intent := VolumeOwnershipIntent{InstanceID: d.ID, ImageSHA256: strings.Repeat("a", 64), UID: d.GuestUID, GID: d.GuestGID, Device: 10, Inode: 100, Size: 16 << 20}
		if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, intent.ImageSHA256, intent.Size); err != nil {
			t.Fatal(err)
		}
		return intent
	}
	intent := makeIntent()
	identityDomain := func() *Domain {
		d := &Domain{ID: intent.InstanceID}
		d.Image.SHA256, d.Image.DataBytes = intent.ImageSHA256, intent.Size
		return d
	}
	if err := m.verifyVolumeOwnershipIntent(ctx, intent); !errors.Is(err, ErrPolicy) {
		t.Fatal("verification created missing intent", err)
	}
	var initialCount int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_volume_ownership`).Scan(&initialCount); err != nil || initialCount != 0 {
		t.Fatal("verification changed inventory", initialCount, err)
	}
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{runtimeMaintenanceKey, "runtime.maintenance-job"} {
		if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, key, "fixture-owner"); err != nil {
			t.Fatal(err)
		}
		for _, check := range []func(context.Context, VolumeOwnershipIntent) error{m.recordVolumeOwnershipIntent, m.verifyVolumeOwnershipIntent} {
			if err := check(ctx, intent); !errors.Is(err, ErrPolicy) {
				t.Fatal("maintenance ownership admission", key, err)
			}
		}
		var savedInode uint64
		if err := m.Store.DB.QueryRow(`SELECT inode FROM runtime_volume_ownership WHERE instance_id=?`, intent.InstanceID).Scan(&savedInode); err != nil || savedInode != intent.Inode {
			t.Fatal("maintenance refusal changed intent", savedInode, err)
		}
		if _, err := m.Store.DB.Exec(`DELETE FROM settings WHERE key=? AND value=?`, key, "fixture-owner"); err != nil {
			t.Fatal(err)
		}
	}
	bound := Domain{ID: intent.InstanceID}
	bound.Image.SHA256, bound.Image.DataBytes = intent.ImageSHA256, intent.Size
	if err := m.bindDomainGuestIdentity(ctx, &bound, false); err != nil {
		t.Fatal("exact ownership domain binding refused", err)
	}
	for _, change := range []func(*Domain){func(d *Domain) { d.Image.SHA256 = strings.Repeat("b", 64) }, func(d *Domain) { d.Image.DataBytes++ }} {
		changed := bound
		changed.GuestUID, changed.GuestGID = 0, 0
		change(&changed)
		if err := m.bindDomainGuestIdentity(ctx, &changed, false); !errors.Is(err, ErrPolicy) || changed.GuestUID != 0 || changed.GuestGID != 0 {
			t.Fatal("ownership/domain drift bound", changed, err)
		}
	}
	replacement := &Manager{Store: m.Store, GuestUIDPool: &pool, GuestGID: m.GuestGID, Policy: m.Policy}
	if err := replacement.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal("immutable retry refused", err)
	}
	if err := m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	m.Store, replacement.Store = reopened, reopened
	if err := replacement.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal("reopened ownership intent refused", err)
	}
	if err := replacement.verifyVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal("saved intent verification refused", err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_uid_leases SET uid=? WHERE instance_id=?`, intent.UID+1, intent.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := m.verifyVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("corrupted saved intent verified: changed durable UID lease admitted")
	}
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("changed durable UID lease admitted")
	}
	if err := m.bindDomainGuestIdentity(ctx, identityDomain(), false); err == nil {
		t.Fatal("binding admitted intent/lease UID mismatch")
	}
	var actualUID uint32
	if err := m.Store.DB.QueryRow(`SELECT uid FROM runtime_uid_leases WHERE instance_id=?`, intent.InstanceID).Scan(&actualUID); err != nil || actualUID != intent.UID+1 {
		t.Fatal("refusal repaired changed lease", actualUID, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_uid_leases SET uid=? WHERE instance_id=?`, intent.UID, intent.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM runtime_guest_groups WHERE instance_id=?`, intent.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := m.verifyVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("corrupted saved intent verified: missing durable group admitted")
	}
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("missing durable group admitted")
	}
	for _, reserve := range []bool{false, true} {
		if err := m.bindDomainGuestIdentity(ctx, identityDomain(), reserve); err == nil {
			t.Fatal("binding admitted missing intent group", reserve)
		}
	}
	var groups int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_guest_groups WHERE instance_id=?`, intent.InstanceID).Scan(&groups); err != nil || groups != 0 {
		t.Fatal("refusal recreated group", groups, err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_guest_groups VALUES(?,?)`, intent.InstanceID, intent.GID); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*VolumeOwnershipIntent){func(i *VolumeOwnershipIntent) { i.Inode++ }, func(i *VolumeOwnershipIntent) { i.Device++ }, func(i *VolumeOwnershipIntent) { i.UID++ }, func(i *VolumeOwnershipIntent) { i.GID++ }, func(i *VolumeOwnershipIntent) { i.Size++ }, func(i *VolumeOwnershipIntent) { i.ImageSHA256 = strings.Repeat("b", 64) }} {
		changed := intent
		change(&changed)
		if err := m.verifyVolumeOwnershipIntent(ctx, changed); err == nil {
			t.Fatal("changed ownership intent verified", changed)
		}
		if err := m.recordVolumeOwnershipIntent(ctx, changed); err == nil {
			t.Fatal("changed ownership intent admitted", changed)
		}
	}
	alias := makeIntent()
	if err := m.recordVolumeOwnershipIntent(ctx, alias); err == nil {
		t.Fatal("same inode assigned to another guest")
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='running' WHERE id=?`, intent.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := m.verifyVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("running-phase mutation authority verified")
	}
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err == nil {
		t.Fatal("running-phase mutation intent admitted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.recordVolumeOwnershipIntent(cancelled, intent); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var count int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_volume_ownership`).Scan(&count); err != nil || count != 1 {
		t.Fatal("refusal changed inventory", count, err)
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM runtime_uid_leases; DELETE FROM runtime_guest_groups; DELETE FROM runtime_uid_pool;`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReserveGuestUID(ctx, state.Random(), pool); err == nil {
		t.Fatal("orphan ownership intent permitted pool recreation")
	}
	if err := m.validateGuestIdentityPolicy(ctx); err == nil {
		t.Fatal("orphan ownership intent admitted at startup")
	}
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_pool`).Scan(&count); err != nil || count != 0 {
		t.Fatal("refusal recreated pool", count, err)
	}
	m.GuestUIDPool, m.GuestGID = nil, 0
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: state.Random()}, false); err == nil {
		t.Fatal("ownership intent allowed shared-identity downgrade")
	}

}

type ownershipAuditBackend struct {
	*fakeBackend
	verifies int
}

func (b *ownershipAuditBackend) Verify(ctx context.Context, d Domain) error {
	b.verifies++
	return b.fakeBackend.Verify(ctx, d)
}

func TestOwnershipMetadataDriftStopsDuringAudit(t *testing.T) {
	for _, field := range []string{"image", "size"} {
		t.Run(field, func(t *testing.T) {
			m, original := newManager(t)
			pool := GuestUIDPool{First: 200000, Last: 200002}
			m.GuestUIDPool, m.GuestGID = &pool, 64055
			backend := &ownershipAuditBackend{fakeBackend: original}
			m.Backend = backend
			ctx := context.Background()
			request := startRequest()
			if _, err := m.Apply(ctx, request); err != nil {
				t.Fatal(err)
			}
			image := m.Manifest.Images[0]
			intent := VolumeOwnershipIntent{InstanceID: request.InstanceID, ImageSHA256: image.SHA256, UID: pool.First, GID: m.GuestGID, Device: 10, Inode: 100, Size: image.DataBytes}
			if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='preparing' WHERE id=?`, request.InstanceID); err != nil {
				t.Fatal(err)
			}
			if err := m.recordVolumeOwnershipIntent(ctx, intent); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='running' WHERE id=?`, request.InstanceID); err != nil {
				t.Fatal(err)
			}
			changed := intent
			if field == "image" {
				changed.ImageSHA256 = strings.Repeat("b", 64)
			} else {
				changed.Size++
			}
			if _, err := m.Store.DB.Exec(`UPDATE runtime_volume_ownership SET image_sha256=?,size=? WHERE instance_id=?`, changed.ImageSHA256, changed.Size, request.InstanceID); err != nil {
				t.Fatal(err)
			}
			backend.verifies = 0
			if err := m.Audit(ctx); err != nil {
				t.Fatal(err)
			}
			if backend.verifies != 0 || backend.stops != 1 || backend.running {
				t.Fatal("drift escaped teardown", backend.verifies, backend.stops, backend.running)
			}
			var phase, desired, digest string
			var size int64
			if err := m.Store.DB.QueryRow(`SELECT state,desired FROM runtime_instances WHERE id=?`, request.InstanceID).Scan(&phase, &desired); err != nil || phase != "interrupted" || desired != "stopped" {
				t.Fatal("teardown state", phase, desired, err)
			}
			if err := m.Store.DB.QueryRow(`SELECT image_sha256,size FROM runtime_volume_ownership WHERE instance_id=?`, request.InstanceID).Scan(&digest, &size); err != nil || digest != changed.ImageSHA256 || size != changed.Size {
				t.Fatal("audit repaired provenance", digest, size, err)
			}
		})
	}
}
