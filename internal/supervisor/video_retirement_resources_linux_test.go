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

func TestNativeVideoRetirementResources(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" || os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") != "1" {
		t.Skip("explicit disposable Linux root fixture required")
	}
	guard := func(context.Context) error { return nil }
	t.Run("complete-physical-sequence-and-replay", func(t *testing.T) {
		m, record, path, stage := nativeVideoPreparedVolumeFixture(t)
		fdCount := func() int {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			return len(entries)
		}
		before := fdCount()
		for i := 0; i < 5; i++ {
			got, err := m.retireVideoResources(context.Background(), record, guard)
			if err != nil || got.Stage != "directory-removed" {
				t.Fatal("physical sequence", got.Stage, err)
			}
			for _, entry := range []string{path, stage, filepath.Join(m.Channels, record.Intent.Request.InstanceID)} {
				if _, err := os.Lstat(entry); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("resource remains", entry, err)
				}
			}
		}
		if fdCount() != before {
			t.Fatal("physical sequence leaked descriptors")
		}
		var leases int
		if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
			t.Fatal("physical sequence released UID", leases, err)
		}
	})
	for _, phase := range []string{"socket-removing", "socket-removed", "volume-removing", "volume-removed", "directory-removing", "directory-removed"} {
		t.Run("reopen-and-resume-from-"+phase, func(t *testing.T) {
			m, record, _, _ := nativeVideoPreparedVolumeFixture(t)
			lost := errors.New("fixture sequence interruption")
			fired := false
			interrupted := func(context.Context) error {
				var current string
				if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil {
					return err
				}
				if !fired && current == phase {
					fired = true
					return lost
				}
				return nil
			}
			got, err := m.retireVideoResources(context.Background(), record, interrupted)
			if !fired || !errors.Is(err, lost) || got.Stage != "" {
				t.Fatal("interruption lost", fired, got.Stage, err)
			}
			// Close the actual journal and discard the in-memory intent. A fresh manager
			// must authenticate the request and reconstruct the pending durable phase.
			request := record.Intent.Request
			record = videoRetirementRecord{}
			if err := m.Store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := state.Open(filepath.Join(m.Images, "journal"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			m = &Manager{Store: reopened, Policy: m.Policy, Backend: m.Backend, GuestUIDPool: m.GuestUIDPool, GuestGID: m.GuestGID, Images: m.Images, Volumes: m.Volumes, Channels: m.Channels}
			record, err = m.prepareVideoRetirement(context.Background(), request, guard)
			if err != nil || record.Stage != phase {
				t.Fatal("reopened journal phase", record.Stage, err)
			}
			next, err := m.retireVideoResources(context.Background(), record, guard)
			if err != nil || next.Stage != "directory-removed" {
				t.Fatal("sequence resume", next.Stage, err)
			}
			var leases int
			if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
				t.Fatal("resume released UID", leases, err)
			}
		})
	}
	t.Run("completed-physical-replay-refuses-recreated-volume", func(t *testing.T) {
		m, record, path, _ := nativeVideoPreparedVolumeFixture(t)
		if _, err := m.retireVideoResources(context.Background(), record, guard); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("preserve replacement"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := m.retireVideoResources(context.Background(), record, guard)
		if !errors.Is(err, ErrPolicy) || got.Stage != "" {
			t.Fatal("replacement accepted", got.Stage, err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "preserve replacement" {
			t.Fatal("replacement changed", err)
		}
	})
	for _, kind := range []string{"channel", "volume"} {
		t.Run("final-replay-refuses-"+kind+"-preparation", func(t *testing.T) {
			m, record, _, _ := nativeVideoPreparedVolumeFixture(t)
			if _, err := m.retireVideoResources(context.Background(), record, guard); err != nil {
				t.Fatal(err)
			}
			parent, suffix := m.Volumes, ".volume-prepare"
			if kind == "channel" {
				parent, suffix = m.Channels, ".channel-prepare"
			}
			path := filepath.Join(parent, "."+record.Intent.Request.InstanceID+suffix)
			if err := os.WriteFile(path, []byte("preserve preparation"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := m.retireVideoResources(context.Background(), record, guard)
			if !errors.Is(err, ErrPolicy) || got.Stage != "" {
				t.Fatal("preparation admitted", got.Stage, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "preserve preparation" {
				t.Fatal("preparation changed", err)
			}
		})
	}
	if !t.Failed() {
		t.Log("reserved video physical sequence component passed; injected stopped guard and retained UID do not qualify complete video cleanup")
	}
}
