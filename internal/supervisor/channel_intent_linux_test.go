//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func testPinnedGuestChannelIntent(t *testing.T) {
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
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, 16<<20); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), d.ID)
	if err := os.Mkdir(path, 0710); err != nil {
		t.Fatal(err)
	}
	open := func(path string) *os.File {
		t.Helper()
		file, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		})
		return file
	}
	directory := open(path)
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, directory); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
		t.Fatal("missing pinned directory provenance admitted", got, err)
	}
	recorded, err := m.recordPinnedChannelOwnership(ctx, d, directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Chown(int(d.GuestUID), 64055); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func(context.Context, Domain, *os.File) (ChannelOwnershipIntent, error){m.recordPinnedChannelOwnership, m.verifyPinnedChannelOwnership} {
		if got, err := check(ctx, d, directory); err != nil || got != recorded {
			t.Fatal("exact directory ownership retry", got, err)
		}
	}
	unknown := filepath.Join(path, "unknown")
	if err := os.WriteFile(unknown, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, directory); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
		t.Fatal("nonempty preparation directory admitted", got, err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "preserve" {
		t.Fatal("refusal changed unknown directory entry", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(filepath.Dir(path), "replacement")
	if err := os.Mkdir(otherPath, 0710); err != nil {
		t.Fatal(err)
	}
	other := open(otherPath)
	if err := other.Chown(int(d.GuestUID), 64055); err != nil {
		t.Fatal(err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, other); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
		t.Fatal("matching ownership adopted another directory", got, err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, directory); err != nil || got != recorded {
		t.Fatal("refusals damaged original directory provenance", got, err)
	}
}
