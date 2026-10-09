//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type maintenanceProbeMutationBackend struct {
	*fakeBackend
	probes int
	mutate func() error
}

func (b *maintenanceProbeMutationBackend) Running(ctx context.Context, id string) (bool, error) {
	b.probes++
	if b.probes == 3 {
		if err := b.mutate(); err != nil {
			return false, err
		}
	}
	return b.fakeBackend.Running(ctx, id)
}

func testReservedMaintenanceDisk(t *testing.T) {
	m, backend := newManager(t)
	ctx := context.Background()
	pool := GuestUIDPool{First: 1000000000, Last: 1000000001}
	m.GuestUIDPool, m.GuestGID = &pool, 64054
	m.Manifest.Images[0].DataBytes = 16 << 20
	d := Domain{ID: state.Random(), Image: m.Manifest.Images[0]}
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	m.Volumes = volumeFixtureDir(t)
	if err := os.Chown(m.Volumes, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(m.Volumes, 0710); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.Volumes, d.ID+".raw")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(d.Image.DataBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("backup-fixture"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,?,?,?,0,1)`, d.ID, d.Image.SHA256, d.Image.MemoryMiB, d.Image.VCPUs, d.Image.DataBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := m.recordPinnedVolumeOwnership(ctx, d, file); err != nil {
		t.Fatal(err)
	}
	if err := file.Chown(int(d.GuestUID), int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='stopped',desired='stopped',revision=2 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	const token = "reserved-backup-fixture"
	if _, err := m.Store.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, runtimeMaintenanceKey, token); err != nil {
		t.Fatal(err)
	}
	called := 0
	if err := m.WithMaintenanceDisk(ctx, token, d.ID, func(ctx context.Context, pinned *os.File, i Instance) error {
		called++
		data := make([]byte, len("backup-fixture"))
		if _, err := pinned.ReadAt(data, 0); err != nil || string(data) != "backup-fixture" {
			t.Fatal("backup data", string(data), err)
		}
		if _, err := pinned.WriteAt([]byte("changed"), 0); err == nil {
			t.Fatal("backup descriptor writable")
		}
		return nil
	}); err != nil || called != 1 {
		t.Fatal("reserved backup admission", called, err)
	}
	if err := m.WithMaintenanceDisk(ctx, token, d.ID, func(context.Context, *os.File, Instance) error { return os.Chmod(path, 0644) }); !errors.Is(err, ErrPolicy) {
		t.Fatal("post-copy metadata drift admitted", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.WithMaintenanceDisk(ctx, token, d.ID, func(context.Context, *os.File, Instance) error {
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("preserve replacement"), 0600)
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("post-copy path replacement admitted", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "preserve replacement" {
		t.Fatal("replacement changed", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".original", path); err != nil {
		t.Fatal(err)
	}
	if err := m.WithMaintenanceDisk(ctx, token, d.ID, func(context.Context, *os.File, Instance) error {
		_, err := m.Store.DB.Exec(`UPDATE settings SET value='changed-owner' WHERE key=?`, runtimeMaintenanceKey)
		return err
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("copy completed after maintenance revocation", err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE settings SET value=? WHERE key=?`, token, runtimeMaintenanceKey); err != nil {
		t.Fatal(err)
	}
	probe := &maintenanceProbeMutationBackend{fakeBackend: backend, mutate: func() error {
		_, err := m.Store.DB.Exec(`UPDATE settings SET value='probe-revocation' WHERE key=?`, runtimeMaintenanceKey)
		return err
	}}
	m.Backend = probe
	if err := m.WithMaintenanceDisk(ctx, token, d.ID, func(context.Context, *os.File, Instance) error { return nil }); !errors.Is(err, ErrPolicy) || probe.probes != 3 {
		t.Fatal("late probe revocation admitted", probe.probes, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE settings SET value=? WHERE key=?`, token, runtimeMaintenanceKey); err != nil {
		t.Fatal(err)
	}
	m.Backend = backend
	called = 0
	if err := m.WithMaintenanceDisk(ctx, "wrong-token", d.ID, func(context.Context, *os.File, Instance) error { called++; return nil }); !errors.Is(err, ErrPolicy) || called != 0 {
		t.Fatal("foreign backup token admitted", called, err)
	}
}
