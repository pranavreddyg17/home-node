//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func nativeVideoDirectoryFixture(t *testing.T) (*Manager, videoRetirementRecord, string, string) {
	t.Helper()
	m, record, _, _ := nativeVideoVolumeFixture(t)
	record, err := m.retireVideoVolume(context.Background(), record, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(m.Channels, record.Intent.Request.InstanceID)
	return m, record, source, filepath.Join(m.Channels, "."+record.Intent.Request.InstanceID+".video-directory-retire-"+strconv.FormatInt(record.Intent.Request.Revision, 10))
}

func TestNativeVideoRetirementDirectoryScope(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" || os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") != "1" {
		t.Skip("explicit disposable Linux root filesystem fixture required")
	}
	guard := func(context.Context) error { return nil }
	t.Run("retire-replay-and-close-descriptors", func(t *testing.T) {
		m, record, source, stage := nativeVideoDirectoryFixture(t)
		count := func() int {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			return len(entries)
		}
		before := count()
		for i := 0; i < 5; i++ {
			next, err := m.retireVideoDirectory(context.Background(), record, guard)
			if err != nil || next.Stage != "directory-removed" {
				t.Fatal(next.Stage, err)
			}
			for _, path := range []string{source, stage} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("directory remains", err)
				}
			}
		}
		if count() != before {
			t.Fatal("directory scope retained descriptors")
		}
		var leases, owners int
		if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
			t.Fatal("UID released", leases, err)
		}
		if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_channel_ownership WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&owners); err != nil || owners != 1 {
			t.Fatal("provenance removed", owners, err)
		}
	})
	for _, point := range []string{"checkpoint", "rename", "rmdir"} {
		t.Run("interruption-"+point, func(t *testing.T) {
			m, record, source, stage := nativeVideoDirectoryFixture(t)
			lost := errors.New("fixture directory interruption")
			fired := false
			interrupted := func(context.Context) error {
				if fired {
					return nil
				}
				_, se := os.Lstat(source)
				_, qe := os.Lstat(stage)
				match := point == "rename" && qe == nil || point == "rmdir" && errors.Is(se, os.ErrNotExist) && errors.Is(qe, os.ErrNotExist)
				if point == "checkpoint" {
					var phase string
					if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&phase); err != nil {
						return err
					}
					match = phase == "directory-removing" && se == nil
				}
				if match {
					fired = true
					return lost
				}
				return nil
			}
			got, err := m.retireVideoDirectory(context.Background(), record, interrupted)
			if !fired || !errors.Is(err, lost) || got.Stage != "" {
				t.Fatal("interruption receipt", fired, got.Stage, err)
			}
			var phase string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&phase); err != nil || phase != "directory-removing" {
				t.Fatal("checkpoint lost", phase, err)
			}
			if next, err := m.retireVideoDirectory(context.Background(), record, guard); err != nil || next.Stage != "directory-removed" {
				t.Fatal("resume failed", next.Stage, err)
			}
		})
	}
	for _, kind := range []string{"replacement", "collision", "symlink", "mode", "foreign-entry", "recreated-volume", "parent-symlink", "unrecorded-absence"} {
		t.Run("refuse-"+kind, func(t *testing.T) {
			m, record, source, stage := nativeVideoDirectoryFixture(t)
			switch kind {
			case "replacement":
				held, err := os.Open(source)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Close()
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(source, 0710); err != nil {
					t.Fatal(err)
				}
				if err := os.Chown(source, int(record.Intent.Ownership.Channel.UID), int(record.Intent.Ownership.Channel.AccessGID)); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(source, 0710); err != nil {
					t.Fatal(err)
				}
			case "collision":
				if err := os.WriteFile(stage, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(m.Volumes, source); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(source, 0700); err != nil {
					t.Fatal(err)
				}
			case "foreign-entry":
				if err := os.WriteFile(filepath.Join(source, "preserve"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "recreated-volume":
				if err := os.WriteFile(filepath.Join(m.Volumes, record.Intent.Request.InstanceID+".raw"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-symlink":
				if err := os.Rename(m.Channels, m.Channels+"-held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(m.Channels+"-held", m.Channels); err != nil {
					t.Fatal(err)
				}
			case "unrecorded-absence":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			}
			var before unix.Stat_t
			beforeErr := unix.Lstat(source, &before)
			got, err := m.retireVideoDirectory(context.Background(), record, guard)
			// A no-follow parent open may return ELOOP/ENOTDIR before journal admission.
			if err == nil || got.Stage != "" {
				t.Fatal("foreign directory admitted", got.Stage, err)
			}
			if kind != "parent-symlink" && !errors.Is(err, ErrPolicy) {
				t.Fatal("unexpected refusal", err)
			}
			var after unix.Stat_t
			afterErr := unix.Lstat(source, &after)
			if beforeErr == nil && (afterErr != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode) {
				t.Fatal("foreign directory changed")
			}
			if errors.Is(beforeErr, unix.ENOENT) && !errors.Is(afterErr, unix.ENOENT) {
				t.Fatal("absent directory adopted")
			}
			var phase string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&phase); err != nil || phase != "volume-removed" {
				t.Fatal("refusal advanced journal", phase, err)
			}
			if kind == "collision" {
				data, err := os.ReadFile(stage)
				if err != nil || string(data) != "preserve" {
					t.Fatal("collision overwritten", err)
				}
			}
		})
	}
	if !t.Failed() {
		t.Log("reserved video directory filesystem component passed; injected stopped guard is not live KVM or UID-release qualification")
	}
}
