package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestChannelLookupRequiresRunningIntent(t *testing.T) {
	m, _ := newManager(t)
	parent, err := os.MkdirTemp("/tmp", "hn-channel-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	m.Channels = parent
	id := state.Random()
	if err := os.Mkdir(filepath.Join(parent, id), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, id, "adapter.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	image := m.Manifest.Images[0]
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','running','stopped',?,?,?,?,0,1)`, id, image.SHA256, image.MemoryMiB, image.VCPUs, image.DataBytes); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Channel(context.Background(), id); !errors.Is(err, ErrPolicy) || got != "" {
		t.Fatal("revoked channel exposed", got, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET desired='running' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Channel(context.Background(), id); err != nil || got != path {
		t.Fatal("running channel refused", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := m.Channel(ctx, id); !errors.Is(err, context.Canceled) || got != "" {
		t.Fatal("canceled lookup returned authority", got, err)
	}
}
