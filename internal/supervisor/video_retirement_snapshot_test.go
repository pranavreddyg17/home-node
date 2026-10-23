package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func retirementSnapshotFixture(t *testing.T) (*Manager, Request) {
	t.Helper()
	m, _ := newManager(t)
	m.GuestUIDPool = &GuestUIDPool{First: 200000, Last: 200002}
	m.GuestGID = 64055
	m.Backend = LinuxBackend{TransferGID: 64056}
	d := Domain{ID: state.Random()}
	if err := m.bindDomainGuestIdentity(context.Background(), &d, true); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'video','stopped','stopped',?,512,1,?,0,2)`, []any{d.ID, digest, 16 << 20}},
		{`INSERT INTO runtime_volume_ownership VALUES(?,?,?,64055,10,100,?)`, []any{d.ID, digest, d.GuestUID, 16 << 20}},
		{`INSERT INTO runtime_channel_ownership VALUES(?,?,?,64055,64056,10,200)`, []any{d.ID, digest, d.GuestUID}},
		{`INSERT INTO runtime_channel_sockets(instance_id,revision,device,inode,retired,retirement_started,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode) VALUES(?,1,10,300,0,0,?,?,64055,64056,10,200)`, []any{d.ID, digest, d.GuestUID}},
	} {
		if _, err := m.Store.DB.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return m, Request{Version: 1, Action: "purge", OperationID: state.Random(), InstanceID: d.ID, Revision: 3, PolicyGeneration: m.Policy.Generation}
}

func TestVideoRetirementSnapshotBindsStoppedOwnershipWithoutGrantingEffects(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	got, err := m.loadVideoRetirementSnapshot(context.Background(), r)
	if err != nil || got.InstanceID != r.InstanceID || got.SourceRevision != 2 || got.Volume.Inode != 100 || got.Channel.Inode != 200 || len(got.Sockets) != 1 || got.Sockets[0].Inode != 300 {
		t.Fatal(got, err)
	}
	var count int
	if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_operations WHERE id=?`, r.OperationID).Scan(&count); err != nil || count != 0 {
		t.Fatal("read created retirement authority", count, err)
	}
	var phase string
	if err := m.Store.DB.QueryRow(`SELECT state FROM runtime_instances WHERE id=?`, r.InstanceID).Scan(&phase); err != nil || phase != "stopped" {
		t.Fatal("snapshot changed runtime", phase, err)
	}
}

func TestVideoRetirementSnapshotRefusesDriftAndForeignWorkloads(t *testing.T) {
	for _, query := range []string{
		`UPDATE runtime_instances SET workload='files'`,
		`UPDATE runtime_instances SET state='running'`,
		`UPDATE runtime_instances SET desired='running'`,
		`UPDATE runtime_instances SET revision=3`,
		`UPDATE runtime_volume_ownership SET uid=200001`,
		`UPDATE runtime_volume_ownership SET inode=0`,
		`UPDATE runtime_channel_ownership SET access_gid=64057`,
		`UPDATE runtime_channel_sockets SET parent_inode=201`,
		`UPDATE runtime_channel_sockets SET revision=3`,
		`DELETE FROM runtime_volume_ownership`,
		`DELETE FROM runtime_channel_sockets`,
		`DELETE FROM runtime_uid_leases`,
		`INSERT INTO settings VALUES('runtime.maintenance','fixture')`,
	} {
		t.Run(query, func(t *testing.T) {
			m, r := retirementSnapshotFixture(t)
			if _, err := m.Store.DB.Exec(query); err != nil {
				t.Fatal(err)
			}
			got, err := m.loadVideoRetirementSnapshot(context.Background(), r)
			if err == nil || got.InstanceID != "" || got.Volume.Inode != 0 || len(got.Sockets) != 0 {
				t.Fatal("drift produced a partial receipt", got, err)
			}
		})
	}
	m, r := retirementSnapshotFixture(t)
	r.PolicyGeneration++
	if _, err := m.loadVideoRetirementSnapshot(context.Background(), r); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign generation", err)
	}
}

func TestVideoRetirementSnapshotBoundsSocketHistory(t *testing.T) {
	m, r := retirementSnapshotFixture(t)
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET revision=258`); err != nil {
		t.Fatal(err)
	}
	r.Revision = 259
	for revision := 2; revision <= 257; revision++ {
		if _, err := m.Store.DB.Exec(`INSERT INTO runtime_channel_sockets(instance_id,revision,device,inode,retired,retirement_started,image_sha256,uid,guest_gid,access_gid,parent_device,parent_inode) VALUES(?,?,10,?,1,1,?,200000,64055,64056,10,200)`, r.InstanceID, revision, 300+revision, strings.Repeat("a", 64)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.loadVideoRetirementSnapshot(context.Background(), r)
	if !errors.Is(err, ErrPolicy) || got.InstanceID != "" || len(got.Sockets) != 0 {
		t.Fatal("unbounded history admitted", got, err)
	}
}
