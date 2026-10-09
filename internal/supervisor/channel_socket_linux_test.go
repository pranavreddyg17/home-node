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
	parent := shortChannelSocketFixtureDir(t)
	channel, err := m.prepareReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, d.ID, "adapter.sock")
	d.ChannelPath = path
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
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='running' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	for retry := 0; retry < 2; retry++ {
		got, err := m.checkPinnedChannelSocket(ctx, 1, channel, directory, socket, false)
		if err != nil || got.Inode != native.Ino {
			t.Fatal("running pinned socket audit", retry, got, err)
		}
	}
	if got, err := m.recordPinnedChannelSocket(ctx, d, 1, directory, socket); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("running audit granted preparation authority", got, err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET state='preparing' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
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
	// An unknown retirement destination must be preserved without a rename.
	stage := filepath.Join(filepath.Dir(path), ".adapter-retire-1")
	if err := os.WriteFile(stage, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.quarantineChannelSocket(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("occupied quarantine adopted", got, err)
	}
	if data, err := os.ReadFile(stage); err != nil || string(data) != "preserve" {
		t.Fatal("quarantine replaced unknown destination", err)
	}
	if err := os.Remove(stage); err != nil {
		t.Fatal(err)
	}
	interruption := errors.New("socket quarantine exclusion lost")
	checks := 0
	got, err := m.quarantineChannelSocket(ctx, parent, d, func(ctx context.Context) error {
		checks++
		if checks == 4 {
			return interruption
		}
		return ctx.Err()
	})
	if !errors.Is(err, interruption) || got != (ChannelSocketIntent{}) {
		t.Fatal("uncertain quarantine reported completion", checks, got, err)
	}
	var moved unix.Stat_t
	if unix.Lstat(stage, &moved) != nil || moved.Ino != native.Ino || moved.Dev != native.Dev || moved.Mode != native.Mode {
		t.Fatal("uncertain quarantine lost admitted socket")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("quarantine left active socket name", err)
	}
	for retry := 0; retry < 2; retry++ {
		got, err := m.quarantineChannelSocket(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
		if err != nil || got.Channel != channel || got.Inode != native.Ino {
			t.Fatal("authenticated quarantine retry", retry, got, err)
		}
	}
	var retired int
	if err := m.Store.DB.QueryRow(`SELECT retired FROM runtime_channel_sockets WHERE instance_id=? AND revision=1`, d.ID).Scan(&retired); err != nil || retired != 0 {
		t.Fatal("quarantine falsely completed retirement", retired, err)
	}

	unknown = filepath.Join(filepath.Dir(path), "preserve-extra")
	if err := os.WriteFile(unknown, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.removeQuarantinedChannelSocket(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("removal admitted unknown directory content", got, err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "preserve" {
		t.Fatal("removal changed unknown content", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	removeChecks := 0
	got, err = m.removeQuarantinedChannelSocket(ctx, parent, d, func(ctx context.Context) error {
		removeChecks++
		if removeChecks == 5 {
			return interruption
		}
		return ctx.Err()
	})
	if !errors.Is(err, interruption) || got != (ChannelSocketIntent{}) {
		t.Fatal("interrupted removal reported completion", removeChecks, got, err)
	}
	if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted removal retained quarantined name", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted removal published active name", err)
	}
	for retry := 0; retry < 2; retry++ {
		got, err := m.removeQuarantinedChannelSocket(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
		if err != nil || got.Channel != channel || got.Inode != native.Ino {
			t.Fatal("authenticated absent removal retry", retry, got, err)
		}
	}
	if err := m.Store.DB.QueryRow(`SELECT retired FROM runtime_channel_sockets WHERE instance_id=? AND revision=1`, d.ID).Scan(&retired); err != nil || retired != 0 {
		t.Fatal("physical removal published unqualified retirement", retired, err)
	}

	expected := ChannelSocketIntent{Channel: channel, Revision: 1, Device: uint64(native.Dev), Inode: native.Ino}
	completionChecks := 0
	got, err = m.retireChannelSocket(ctx, parent, d, expected, func(ctx context.Context) error {
		completionChecks++
		if completionChecks == 6 {
			return interruption
		}
		return ctx.Err()
	})
	if !errors.Is(err, interruption) || got != (ChannelSocketIntent{}) {
		t.Fatal("interrupted completion reported authority", completionChecks, got, err)
	}
	if err := m.Store.DB.QueryRow(`SELECT retired FROM runtime_channel_sockets WHERE instance_id=? AND revision=1`, d.ID).Scan(&retired); err != nil || retired != 1 {
		t.Fatal("qualified retirement not preserved", retired, err)
	}
	for retry := 0; retry < 2; retry++ {
		got, err := m.retireChannelSocket(ctx, parent, d, expected, func(ctx context.Context) error { return ctx.Err() })
		if err != nil || got != expected {
			t.Fatal("completed retirement retry", retry, got, err)
		}
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET revision=2 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	replacement.SetUnlinkOnClose(false)
	defer replacement.Close()
	if err := os.Chown(path, int(d.GuestUID), 64055); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	newSocket, err := os.OpenFile(path, unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer newSocket.Close()
	newIntent, err := m.recordPinnedChannelSocket(ctx, d, 2, directory, newSocket)
	if err != nil || newIntent.Revision != 2 {
		t.Fatal("new revision after completed retirement", newIntent, err)
	}
	if got, err := m.retireChannelSocket(ctx, parent, d, expected, func(ctx context.Context) error { return ctx.Err() }); !errors.Is(err, ErrPolicy) || got != (ChannelSocketIntent{}) {
		t.Fatal("old retirement retry touched newer socket", got, err)
	}
	var preserved unix.Stat_t
	if unix.Lstat(path, &preserved) != nil || preserved.Ino != newIntent.Inode || uint64(preserved.Dev) != newIntent.Device {
		t.Fatal("old retirement lost new socket")
	}
	if err := m.verifyChannelSocketIntent(ctx, newIntent); err != nil {
		t.Fatal("old retirement changed new provenance", err)
	}

}

// Leave room for the instance ID and socket name within sockaddr_un.sun_path.
func shortChannelSocketFixtureDir(t *testing.T) string {
	t.Helper()
	path, err := os.MkdirTemp("", "hn-socket-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Error(err)
		}
	})
	return path
}
