package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestChannelSocketIntentBindsRuntimeRevision(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200002}
	m.GuestUIDPool, m.GuestGID = &pool, 64054
	m.Backend = LinuxBackend{TransferGID: 64055}
	ctx := context.Background()
	d := Domain{ID: state.Random()}
	d.Image.SHA256 = strings.Repeat("a", 64)
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,1)`, d.ID, d.Image.SHA256, 16<<20); err != nil {
		t.Fatal(err)
	}
	channel := ChannelOwnershipIntent{InstanceID: d.ID, ImageSHA256: d.Image.SHA256, UID: d.GuestUID, GuestGID: d.GuestGID, AccessGID: 64055, Device: 10, Inode: 100}
	if err := m.recordChannelOwnershipIntent(ctx, channel); err != nil {
		t.Fatal(err)
	}
	intent := ChannelSocketIntent{Channel: channel, Revision: 1, Device: 10, Inode: 101}
	if err := m.verifyChannelSocketIntent(ctx, intent); !errors.Is(err, ErrPolicy) {
		t.Fatal("verification created missing socket proof", err)
	}
	if err := m.recordChannelSocketIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='running' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := m.loadActiveChannelSocketIntent(ctx, d, 1); err != nil || got != intent {
		t.Fatal("running socket receipt", got, err)
	}
	if _, err := m.loadActiveChannelSocketIntent(ctx, d, 2); !errors.Is(err, ErrPolicy) {
		t.Fatal("missing audit receipt admitted", err)
	}
	changedDomain := d
	changedDomain.GuestUID++
	if _, err := m.loadActiveChannelSocketIntent(ctx, changedDomain, 1); !errors.Is(err, ErrPolicy) {
		t.Fatal("caller identity drift admitted", err)
	}
	if _, err := m.loadChannelOwnershipIntent(ctx, d); !errors.Is(err, ErrPolicy) {
		t.Fatal("running audit granted preparation authority", err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='preparing' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	// Upgrade the original socket schema without replacing its active proof.
	if _, err := m.Store.DB.Exec(`ALTER TABLE runtime_channel_sockets DROP COLUMN retirement_started`); err != nil {
		t.Fatal(err)
	}
	if err := m.initializeGuestUIDLeases(ctx); err != nil {
		t.Fatal("socket retirement schema migration", err)
	}
	if err := m.verifyChannelSocketIntent(ctx, intent); err != nil {
		t.Fatal("migration changed active proof", err)
	}
	if err := m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	m.Store = reopened
	for _, check := range []func(context.Context, ChannelSocketIntent) error{m.recordChannelSocketIntent, m.verifyChannelSocketIntent} {
		if err := check(ctx, intent); err != nil {
			t.Fatal("reopened socket proof", err)
		}
		for _, change := range []func(*ChannelSocketIntent){func(i *ChannelSocketIntent) { i.Inode++ }, func(i *ChannelSocketIntent) { i.Device++ }, func(i *ChannelSocketIntent) { i.Revision++ }, func(i *ChannelSocketIntent) { i.Channel.Inode++ }, func(i *ChannelSocketIntent) { i.Channel.AccessGID++ }} {
			changed := intent
			change(&changed)
			if err := check(ctx, changed); !errors.Is(err, ErrPolicy) {
				t.Fatal("changed socket proof admitted", changed, err)
			}
		}
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_channel_ownership SET inode=inode+1 WHERE instance_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.validateGuestIdentityPolicy(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("socket parent provenance drift admitted", err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_channel_ownership SET inode=inode-1 WHERE instance_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET revision=2 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	next := intent
	next.Revision = 2
	next.Inode++
	if err := m.recordChannelSocketIntent(ctx, next); !errors.Is(err, ErrPolicy) {
		t.Fatal("new revision bypassed active socket retirement", err)
	}
	if err := m.verifyChannelSocketIntent(ctx, intent); !errors.Is(err, ErrPolicy) {
		t.Fatal("old revision admitted for active use", err)
	}
	if got, err := m.loadChannelSocketRetirementIntent(ctx, d); err != nil || got != intent {
		t.Fatal("historical retirement receipt", got, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='running' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := m.loadChannelSocketRetirementIntent(ctx, d); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("running guest obtained retirement receipt", got, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='preparing' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, runtimeMaintenanceKey, "fixture"); err != nil {
		t.Fatal(err)
	}
	if got, err := m.loadChannelSocketRetirementIntent(ctx, d); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("maintenance retirement admission", got, err)
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM settings WHERE key=?`, runtimeMaintenanceKey); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := m.loadChannelSocketRetirementIntent(canceled, d); !errors.Is(err, context.Canceled) || got != (ChannelSocketIntent{}) {
		t.Fatal("canceled retirement receipt", got, err)
	}
	interruption := errors.New("retirement exclusion lost")
	checks := 0
	got, err := m.beginChannelSocketRetirement(ctx, d, func(ctx context.Context) error {
		checks++
		if checks == 2 {
			return interruption
		}
		return ctx.Err()
	})
	if !errors.Is(err, interruption) || got != (ChannelSocketIntent{}) {
		t.Fatal("uncertain retirement checkpoint reported authority", got, err)
	}
	var started int
	if err := m.Store.DB.QueryRow(`SELECT retirement_started FROM runtime_channel_sockets WHERE instance_id=? AND revision=?`, d.ID, intent.Revision).Scan(&started); err != nil || started != 1 {
		t.Fatal("uncertain checkpoint not preserved", started, err)
	}
	if err := m.Store.Close(); err != nil {
		t.Fatal(err)
	}
	afterCheckpoint, err := state.Open(filepath.Join(m.Images, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = afterCheckpoint.Close() })
	m.Store = afterCheckpoint
	for retry := 0; retry < 2; retry++ {
		got, err := m.beginChannelSocketRetirement(ctx, d, func(ctx context.Context) error { return ctx.Err() })
		if err != nil || got != intent {
			t.Fatal("reopened retirement checkpoint", retry, got, err)
		}
	}
	if err := m.recordChannelSocketIntent(ctx, next); !errors.Is(err, ErrPolicy) {
		t.Fatal("checkpoint bypassed physical retirement", err)
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM runtime_channel_ownership WHERE instance_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.validateGuestIdentityPolicy(ctx); !errors.Is(err, ErrPolicy) {
		t.Fatal("orphan socket proof admitted at startup", err)
	}
}
