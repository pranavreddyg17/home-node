package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceVolumeIntentAdmission(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200002}
	m.GuestUIDPool, m.GuestGID = &pool, 64054
	ctx := context.Background()
	d := Domain{ID: state.Random()}
	d.Image.SHA256, d.Image.DataBytes = strings.Repeat("a", 64), 16<<20
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,1)`, d.ID, d.Image.SHA256, d.Image.DataBytes); err != nil {
		t.Fatal(err)
	}
	intent := VolumeOwnershipIntent{InstanceID: d.ID, ImageSHA256: d.Image.SHA256, UID: d.GuestUID, GID: d.GuestGID, Device: 10, Inode: 100, Size: d.Image.DataBytes}
	if err := m.recordVolumeOwnershipIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='stopped',desired='stopped',revision=2 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	const token = "owned-maintenance-token"
	if _, err := m.loadMaintenanceVolumeIntent(ctx, token, d, 2); !errors.Is(err, ErrPolicy) {
		t.Fatal("missing barrier admitted", err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, runtimeMaintenanceKey, token); err != nil {
		t.Fatal(err)
	}
	for retry := 0; retry < 2; retry++ {
		if got, err := m.loadMaintenanceVolumeIntent(ctx, token, d, 2); err != nil || got != intent {
			t.Fatal("saved stopped volume", retry, got, err)
		}
	}
	for _, bad := range []struct {
		token    string
		revision int64
	}{{"wrong", 2}, {token, 1}} {
		if got, err := m.loadMaintenanceVolumeIntent(ctx, bad.token, d, bad.revision); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("wrong maintenance authority", got, err)
		}
	}
	for _, change := range []string{"state='running'", "desired='running'", "data_bytes=data_bytes+1", "image_sha256='changed'"} {
		if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET `+change+` WHERE id=?`, d.ID); err != nil {
			t.Fatal(err)
		}
		if got, err := m.loadMaintenanceVolumeIntent(ctx, token, d, 2); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("runtime drift admitted", change, got, err)
		}
		if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='stopped',desired='stopped',data_bytes=?,image_sha256=? WHERE id=?`, d.Image.DataBytes, d.Image.SHA256, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Store.DB.Exec(`DELETE FROM runtime_volume_ownership WHERE instance_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := m.loadMaintenanceVolumeIntent(ctx, token, d, 2); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
		t.Fatal("missing receipt repaired", got, err)
	}
	var count int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_volume_ownership`).Scan(&count); err != nil || count != 0 {
		t.Fatal("maintenance created provenance", count, err)
	}
}
