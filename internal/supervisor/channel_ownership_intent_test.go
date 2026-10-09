package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestChannelOwnershipIntentBindsLeaseAndDirectory(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200003}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	m.Backend = LinuxBackend{TransferGID: 64056}
	ctx := context.Background()
	d := Domain{ID: state.Random()}
	d.Image.SHA256 = strings.Repeat("a", 64)
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, 16<<20); err != nil {
		t.Fatal(err)
	}
	intent := ChannelOwnershipIntent{InstanceID: d.ID, ImageSHA256: d.Image.SHA256, UID: d.GuestUID, GuestGID: d.GuestGID, AccessGID: 64056, Device: 10, Inode: 100}
	if err := m.verifyChannelOwnershipIntent(ctx, intent); !errors.Is(err, ErrPolicy) {
		t.Fatal("verification adopted missing directory intent", err)
	}
	var count int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_channel_ownership`).Scan(&count); err != nil || count != 0 {
		t.Fatal("verification created provenance", count, err)
	}
	if err := m.recordChannelOwnershipIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	replacement := &Manager{Store: m.Store, GuestUIDPool: &pool, GuestGID: m.GuestGID, Policy: m.Policy, Backend: m.Backend}
	if err := m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	m.Store, replacement.Store = reopened, reopened
	for _, manager := range []*Manager{m, replacement} {
		if err := manager.recordChannelOwnershipIntent(ctx, intent); err != nil {
			t.Fatal("immutable restart retry", err)
		}
		if err := manager.verifyChannelOwnershipIntent(ctx, intent); err != nil {
			t.Fatal("restart authentication", err)
		}
		if loaded, err := manager.loadChannelOwnershipIntent(ctx, d); err != nil || loaded != intent {
			t.Fatal("reopened directory intent loading", loaded, err)
		}
	}
	for _, change := range []func(*Domain){
		func(d *Domain) { d.GuestUID++ },
		func(d *Domain) { d.GuestGID++ },
		func(d *Domain) { d.Image.SHA256 = strings.Repeat("b", 64) },
	} {
		changed := d
		change(&changed)
		if loaded, err := m.loadChannelOwnershipIntent(ctx, changed); !errors.Is(err, ErrPolicy) || loaded != (ChannelOwnershipIntent{}) {
			t.Fatal("foreign domain loaded directory authority", loaded, err)
		}
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, runtimeMaintenanceKey, "fixture-owner"); err != nil {
		t.Fatal(err)
	}
	if loaded, err := m.loadChannelOwnershipIntent(ctx, d); !errors.Is(err, ErrPolicy) || loaded != (ChannelOwnershipIntent{}) {
		t.Fatal("maintenance admitted directory intent", loaded, err)
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM settings WHERE key=?`, runtimeMaintenanceKey); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if loaded, err := m.loadChannelOwnershipIntent(canceled, d); !errors.Is(err, context.Canceled) || loaded != (ChannelOwnershipIntent{}) {
		t.Fatal("canceled loading returned directory authority", loaded, err)
	}
	for _, change := range []func(*ChannelOwnershipIntent){
		func(i *ChannelOwnershipIntent) { i.Inode++ },
		func(i *ChannelOwnershipIntent) { i.Device++ },
		func(i *ChannelOwnershipIntent) { i.UID++ },
		func(i *ChannelOwnershipIntent) { i.GuestGID++ },
		func(i *ChannelOwnershipIntent) { i.AccessGID++ },
		func(i *ChannelOwnershipIntent) { i.ImageSHA256 = strings.Repeat("b", 64) },
	} {
		changed := intent
		change(&changed)
		for _, check := range []func(context.Context, ChannelOwnershipIntent) error{m.recordChannelOwnershipIntent, m.verifyChannelOwnershipIntent} {
			if err := check(ctx, changed); !errors.Is(err, ErrPolicy) {
				t.Fatal("changed directory authority admitted", changed, err)
			}
		}
	}
	m.Backend = LinuxBackend{TransferGID: 64057}
	if err := m.validateGuestIdentityPolicy(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("changed channel access group accepted", err)
	}
	m.Backend = replacement.Backend
	if _, err := m.Store.DB.Exec(`DELETE FROM runtime_guest_groups WHERE instance_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReserveGuestUID(ctx, state.Random(), pool); !errors.Is(err, ErrPolicy) {
		t.Fatal("orphan channel admitted new allocation", err)
	}
	rebound := Domain{ID: d.ID}
	rebound.Image = d.Image
	if err := m.bindDomainGuestIdentity(ctx, &rebound, true); !errors.Is(err, ErrPolicy) || rebound.GuestUID != 0 || rebound.GuestGID != 0 {
		t.Fatal("orphan channel repaired group assignment", rebound, err)
	}
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_guest_groups WHERE instance_id=?`, d.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("refusal repaired missing group", count, err)
	}
}
