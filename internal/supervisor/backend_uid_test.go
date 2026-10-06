package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestSharedPreparationRefusesReservedDACBeforeEffects(t *testing.T) {
	root := t.TempDir()
	id := state.Random()
	if !guestproto.ValidID(id) {
		t.Fatal("invalid test identity")
	}
	channel := filepath.Join(root, "channels", id, "adapter.sock")
	d := Domain{ID: id, DataPath: filepath.Join(root, id+".raw"), ChannelPath: channel, DiskReserveBytes: 4 << 30}
	for _, identity := range [][2]uint32{{200000, 200000}, {200000, 0}, {0, 200000}} {
		d.GuestUID, d.GuestGID = identity[0], identity[1]
		if err := (LinuxBackend{}).Prepare(context.Background(), d); !errors.Is(err, ErrPolicy) {
			t.Fatal("unsupported ownership lifecycle accepted", identity, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("refused preparation changed files", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (LinuxBackend{}).Prepare(ctx, d); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled preparation ignored", err)
	}
}
