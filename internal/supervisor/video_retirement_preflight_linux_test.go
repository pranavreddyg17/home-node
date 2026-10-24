//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeVideoRetirementPreflight(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" || os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") != "1" {
		t.Skip("explicit disposable Linux root fixture required")
	}
	for _, kind := range []string{"replacement", "symlink", "hardlink", "mode", "size", "missing", "quarantine-collision", "preparation-collision", "channel-preparation-collision"} {
		t.Run("refuse-volume-before-socket-effects-"+kind, func(t *testing.T) {
			m, record, path, stage := nativeVideoPreparedVolumeFixture(t)
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
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(m.Channels, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+"-alias"); err != nil {
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
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "quarantine-collision":
				if err := os.WriteFile(stage, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "channel-preparation-collision":
				if err := os.WriteFile(filepath.Join(m.Channels, "."+record.Intent.Request.InstanceID+".channel-prepare"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "preparation-collision":
				if err := os.WriteFile(filepath.Join(m.Volumes, "."+record.Intent.Request.InstanceID+".volume-prepare"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			socket := filepath.Join(m.Channels, record.Intent.Request.InstanceID, "adapter.sock")
			var before unix.Stat_t
			if unix.Lstat(socket, &before) != nil {
				t.Fatal("fixture socket absent")
			}
			got, err := m.retireVideoSocketWithVolume(context.Background(), record, func(context.Context) error { return nil })
			if !errors.Is(err, ErrPolicy) || got.Stage != "" {
				t.Fatal("foreign volume admitted", got.Stage, err)
			}
			var after unix.Stat_t
			if unix.Lstat(socket, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Nlink != after.Nlink {
				t.Fatal("volume refusal changed socket")
			}
			var phase string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&phase); err != nil || phase != "prepared" {
				t.Fatal("volume refusal advanced journal", phase, err)
			}
		})
	}
	t.Run("retains-volume-guard-at-socket-checkpoint", func(t *testing.T) {
		m, record, path, _ := nativeVideoPreparedVolumeFixture(t)
		changed := false
		guard := func(context.Context) error {
			var phase string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&phase); err != nil {
				return err
			}
			if phase == "socket-removing" && !changed {
				changed = true
				return os.Chmod(path, 0640)
			}
			return nil
		}
		got, err := m.retireVideoSocketWithVolume(context.Background(), record, guard)
		if !changed || !errors.Is(err, ErrPolicy) || got.Stage != "" {
			t.Fatal("lost volume guard admitted", changed, got.Stage, err)
		}
		if _, err := os.Lstat(filepath.Join(m.Channels, record.Intent.Request.InstanceID, "adapter.sock")); err != nil {
			t.Fatal("socket changed after volume guard lost", err)
		}
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
		next, err := m.retireVideoSocketWithVolume(context.Background(), record, func(context.Context) error { return nil })
		if err != nil || next.Stage != "socket-removed" {
			t.Fatal("preflight retry", next.Stage, err)
		}
	})
	if !t.Failed() {
		t.Log("reserved video volume preflight component passed; injected stopped guard does not qualify live process exclusion or UID release")
	}
}
