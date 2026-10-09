//go:build linux

package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func testPinnedChannelSocket(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 1000000000, Last: 1000000001}
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
	parent := t.TempDir()
	channel, err := m.prepareReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, d.ID, "adapter.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	if err := os.Chown(path, int(d.GuestUID), 64055); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	socket, err := os.OpenFile(path, unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	var native unix.Stat_t
	if unix.Fstat(int(socket.Fd()), &native) != nil {
		t.Fatal("socket metadata unavailable")
	}
	for retry := 0; retry < 2; retry++ {
		got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket)
		if err != nil || got.Channel != channel || got.Inode != native.Ino || got.Device != uint64(native.Dev) {
			t.Fatal("pinned socket record", retry, got, err)
		}
	}
	unknown := filepath.Join(filepath.Dir(path), "unknown")
	if err := os.WriteFile(unknown, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("unknown channel entry admitted", got, err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "preserve" {
		t.Fatal("refusal changed unknown entry", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("socket mode drift admitted", got, err)
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	alias := path + ".alias"
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("linked socket admitted", got, err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket); err != nil || got.Inode != native.Ino {
		t.Fatal("refusal damaged socket provenance", got, err)
	}
}
