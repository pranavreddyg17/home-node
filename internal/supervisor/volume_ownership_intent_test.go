package supervisor

import (
	"context"
	"errors"
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
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	replacement := &Manager{Store: m.Store, GuestUIDPool: &pool, GuestGID: m.GuestGID, Policy: m.Policy}
	if err := replacement.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal("immutable retry refused", err)
	}
	for _, change := range []func(*VolumeOwnershipIntent){func(i *VolumeOwnershipIntent) { i.Inode++ }, func(i *VolumeOwnershipIntent) { i.Device++ }, func(i *VolumeOwnershipIntent) { i.UID++ }, func(i *VolumeOwnershipIntent) { i.GID++ }, func(i *VolumeOwnershipIntent) { i.Size++ }, func(i *VolumeOwnershipIntent) { i.ImageSHA256 = strings.Repeat("b", 64) }} {
		changed := intent
		change(&changed)
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
}
