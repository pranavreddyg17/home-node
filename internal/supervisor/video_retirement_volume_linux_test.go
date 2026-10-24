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

func nativeRetirementVolume(t *testing.T, path string, owner VolumeOwnershipIntent) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("generated private video volume\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(owner.Size); err != nil {
		t.Fatal(err)
	}
	if err := file.Chown(int(owner.UID), int(owner.GID)); err != nil {
		t.Fatal(err)
	}
	if err := file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
}

func nativeVideoVolumeFixture(t *testing.T) (*Manager, videoRetirementRecord, string, string) {
	t.Helper()
	m, r, _, _ := nativeVideoSocketResources(t)
	if err := os.Chown(m.Volumes, 0, int(m.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(m.Volumes, 0710); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.Volumes, r.InstanceID+".raw")
	nativeRetirementVolume(t, path, VolumeOwnershipIntent{UID: 200000, GID: m.GuestGID, Size: 16 << 20})
	var metadata unix.Stat_t
	if unix.Lstat(path, &metadata) != nil {
		t.Fatal("volume stat")
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_volume_ownership SET device=?,inode=? WHERE instance_id=?`, uint64(metadata.Dev), metadata.Ino, r.InstanceID); err != nil {
		t.Fatal(err)
	}
	record, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	record, err = m.retireVideoSocket(context.Background(), record, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return m, record, path, filepath.Join(m.Volumes, "."+r.InstanceID+".video-volume-retire-"+strconv.FormatInt(r.Revision, 10))
}

func TestNativeVideoRetirementVolumeScope(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" || os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") != "1" {
		t.Skip("explicit disposable Linux root filesystem fixture required")
	}
	guard := func(context.Context) error { return nil }
	t.Run("retire-replay-and-close-descriptors", func(t *testing.T) {
		m, record, path, stage := nativeVideoVolumeFixture(t)
		countFDs := func() int {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			return len(entries)
		}
		before := countFDs()
		for attempt := 0; attempt < 5; attempt++ {
			next, err := m.retireVideoVolume(context.Background(), record, guard)
			if err != nil || next.Stage != "volume-removed" {
				t.Fatal(next.Stage, err)
			}
			for _, entry := range []string{path, stage} {
				if _, err := os.Lstat(entry); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("volume remains", entry, err)
				}
			}
		}
		if after := countFDs(); after != before {
			t.Fatal("volume scope retained descriptors", before, after)
		}
		var leases int
		if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
			t.Fatal("volume retirement released UID", leases, err)
		}
		if _, err := os.Lstat(filepath.Join(m.Channels, record.Intent.Request.InstanceID)); err != nil {
			t.Fatal("volume scope removed channel directory", err)
		}
	})
	for _, point := range []string{"checkpoint", "rename", "unlink"} {
		t.Run("interruption-"+point, func(t *testing.T) {
			m, record, path, stage := nativeVideoVolumeFixture(t)
			lost := errors.New("fixture volume interruption")
			fired := false
			interrupted := func(context.Context) error {
				if fired {
					return nil
				}
				_, sourceErr := os.Lstat(path)
				_, stageErr := os.Lstat(stage)
				match := point == "rename" && stageErr == nil || point == "unlink" && errors.Is(sourceErr, os.ErrNotExist) && errors.Is(stageErr, os.ErrNotExist)
				if point == "checkpoint" {
					var current string
					if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil {
						return err
					}
					match = current == "volume-removing" && sourceErr == nil
				}
				if match {
					fired = true
					return lost
				}
				return nil
			}
			got, err := m.retireVideoVolume(context.Background(), record, interrupted)
			if !fired || !errors.Is(err, lost) || got.Stage != "" {
				t.Fatal("volume interruption not retained", fired, got.Stage, err)
			}
			var current string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil || current != "volume-removing" {
				t.Fatal("volume checkpoint lost", current, err)
			}
			if next, err := m.retireVideoVolume(context.Background(), record, guard); err != nil || next.Stage != "volume-removed" {
				t.Fatal("volume interruption did not reconcile", next.Stage, err)
			}
		})
	}
	for _, kind := range []string{"replacement", "collision", "symlink", "hardlink", "mode", "size", "recreated-socket", "parent-symlink"} {
		t.Run("refuse-"+kind, func(t *testing.T) {
			m, record, path, stage := nativeVideoVolumeFixture(t)
			switch kind {
			case "replacement":
				fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer unix.Close(fd)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				nativeRetirementVolume(t, path, record.Intent.Ownership.Volume)
			case "collision":
				if err := os.WriteFile(stage, []byte("preserve foreign stage"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(filepath.Dir(m.Volumes), "preserve-target")
				if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(m.Volumes, "foreign-alias")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
			case "size":
				if err := os.Truncate(path, 8<<20); err != nil {
					t.Fatal(err)
				}
			case "recreated-socket":
				nativeRetirementSocket(t, filepath.Join(m.Channels, record.Intent.Request.InstanceID, "adapter.sock"), record.Intent.Ownership.Channel.UID, record.Intent.Ownership.Channel.AccessGID)
			case "parent-symlink":
				moved := m.Volumes + "-original"
				if err := os.Rename(m.Volumes, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, m.Volumes); err != nil {
					t.Fatal(err)
				}
			}
			var before unix.Stat_t
			if unix.Lstat(path, &before) != nil {
				t.Fatal("fixture volume absent")
			}
			got, err := m.retireVideoVolume(context.Background(), record, guard)
			if !errors.Is(err, ErrPolicy) || got.Stage != "" {
				t.Fatal("foreign volume admitted", got.Stage, err)
			}
			var after unix.Stat_t
			if unix.Lstat(path, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Size != after.Size {
				t.Fatal("foreign volume changed")
			}
			var current string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil || current != "socket-removed" {
				t.Fatal("failed volume admission advanced journal", current, err)
			}
			if kind == "collision" {
				contents, err := os.ReadFile(stage)
				if err != nil || string(contents) != "preserve foreign stage" {
					t.Fatal("foreign volume stage overwritten", err)
				}
			}
		})
	}
	if !t.Failed() {
		t.Log("reserved video volume filesystem component passed; injected stopped guard is not live KVM or UID-release qualification")
	}
}
