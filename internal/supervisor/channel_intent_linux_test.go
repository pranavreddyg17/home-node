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
	// The shipped supervisor mask is 0077. Establish the fixture's exact
	// intended mode through its retained descriptor before strict admission.
	if err := directory.Chmod(0710); err != nil {
		t.Fatal(err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, directory); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
		t.Fatal("missing pinned directory provenance admitted", got, err)
	}
	recorded, err := m.recordPinnedChannelOwnership(ctx, d, directory)
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	interruption := errors.New("channel runtime exclusion lost")
	transferred, transferErr := m.transferReservedChannel(ctx, path, d, func(ctx context.Context) error {
		checks++
		if checks == 4 {
			return interruption
		}
		return ctx.Err()
	})
	if !errors.Is(transferErr, interruption) || transferred != (ChannelOwnershipIntent{}) {
		t.Fatal("uncertain channel transfer reported success", transferred, transferErr)
	}
	var changed unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &changed) != nil || changed.Ino != recorded.Inode || changed.Uid != d.GuestUID || changed.Gid != recorded.AccessGID {
		t.Fatal("uncertain channel effects did not preserve exact directory")
	}
	stopped := func(ctx context.Context) error { return ctx.Err() }
	for retry := 0; retry < 2; retry++ {
		transferred, transferErr = m.transferReservedChannel(ctx, path, d, stopped)
		if transferErr != nil || transferred != recorded {
			t.Fatal("authenticated channel transfer retry", retry, transferred, transferErr)
		}
	}
	// Completion must remain guest-owned through both post-transfer guards.
	for _, revertAt := range []int{4, 5} {
		checks := 0
		got, err := m.transferReservedChannel(ctx, path, d, func(ctx context.Context) error {
			checks++
			if checks == revertAt {
				var owner unix.Stat_t
				if unix.Fstat(int(directory.Fd()), &owner) != nil || owner.Uid != d.GuestUID {
					t.Fatal("fault did not follow ownership transfer", revertAt)
				}
				return directory.Chown(0, 0)
			}
			return ctx.Err()
		})
		if !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
			t.Fatal("reverted ownership reported completion", revertAt, got, err)
		}
		got, err = m.transferReservedChannel(ctx, path, d, stopped)
		if err != nil || got != recorded {
			t.Fatal("ownership reversion retry", revertAt, got, err)
		}
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
	if err := other.Chmod(0710); err != nil {
		t.Fatal(err)
	}
	if err := other.Chown(int(d.GuestUID), 64055); err != nil {
		t.Fatal(err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, other); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
		t.Fatal("matching ownership adopted another directory", got, err)
	}
	if got, err := m.verifyPinnedChannelOwnership(ctx, d, directory); err != nil || got != recorded {
		t.Fatal("refusals damaged original directory provenance", got, err)
	}
	if err := m.withReservedChannel(ctx, path, d, stopped, func(ctx context.Context, pinned *os.File, intent ChannelOwnershipIntent, guard func(context.Context) error) error {
		moved := path + ".displaced"
		if err := os.Rename(path, moved); err != nil {
			return err
		}
		if err := os.Symlink(moved, path); err != nil {
			return errors.Join(err, os.Rename(moved, path))
		}
		guardErr := guard(ctx)
		if err := errors.Join(os.Remove(path), os.Rename(moved, path)); err != nil {
			t.Fatal("restore redirected channel fixture", err)
		}
		if !errors.Is(guardErr, ErrPolicy) {
			t.Fatal("scope accepted redirected channel directory", guardErr)
		}
		return guardErr
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("scope lost directory redirection refusal", err)
	}
}
