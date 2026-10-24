//go:build linux

package supervisor

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func nativeRetirementSocket(t *testing.T, path string, uid, gid uint32) {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, int(uid), int(gid)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
}

func nativeVideoSocketResources(t *testing.T) (*Manager, Request, string, string) {
	t.Helper()
	m, r := retirementSnapshotFixture(t)
	// Keep Unix names short and all fixture objects beneath a root-private parent.
	base, err := os.MkdirTemp("/tmp", "hn-vrs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	})
	m.Channels = filepath.Join(base, "c")
	if err := os.Mkdir(m.Channels, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(m.Channels, 0, 64056); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(m.Channels, 0710); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(m.Channels, r.InstanceID)
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 200000, 64056); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "adapter.sock")
	nativeRetirementSocket(t, path, 200000, 64056)
	var parent, child unix.Stat_t
	if unix.Lstat(directory, &parent) != nil || unix.Lstat(path, &child) != nil {
		t.Fatal("fixture stat")
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_channel_ownership SET device=?,inode=? WHERE instance_id=?`, uint64(parent.Dev), parent.Ino, r.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec(`UPDATE runtime_channel_sockets SET device=?,inode=?,parent_device=?,parent_inode=? WHERE instance_id=?`, uint64(child.Dev), child.Ino, uint64(parent.Dev), parent.Ino, r.InstanceID); err != nil {
		t.Fatal(err)
	}
	return m, r, path, filepath.Join(directory, ".adapter-video-retire-"+strconv.FormatInt(r.Revision, 10))
}

func nativeVideoSocketFixture(t *testing.T) (*Manager, videoRetirementRecord, string, string) {
	t.Helper()
	m, r, path, stage := nativeVideoSocketResources(t)
	record, err := m.prepareVideoRetirement(context.Background(), r, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return m, record, path, stage
}

func TestNativeVideoRetirementSocketScope(t *testing.T) {
	if os.Geteuid() != 0 || (os.Getenv("HOMENODE_VIDEO_RETIREMENT_FS_INTEGRATION") != "1" || os.Getenv("HOMENODE_VIDEO_RETIREMENT_HARDLINK_NEGATIVE") != "1") {
		t.Skip("explicit disposable Linux root filesystem fixture required")
	}
	guard := func(context.Context) error { return nil }
	t.Run("retire-replay-and-close-descriptors", func(t *testing.T) {
		m, record, path, stage := nativeVideoSocketFixture(t)
		countFDs := func() int {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			return len(entries)
		}
		before := countFDs()
		for attempt := 0; attempt < 5; attempt++ {
			next, err := m.retireVideoSocket(context.Background(), record, guard)
			if err != nil || next.Stage != "socket-removed" {
				t.Fatal(next.Stage, err)
			}
			for _, name := range []string{path, stage} {
				if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("socket remains", name, err)
				}
			}
		}
		if after := countFDs(); after != before {
			t.Fatal("scope retained descriptors", before, after)
		}
		var leases int
		if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_uid_leases WHERE instance_id=?`, record.Intent.Request.InstanceID).Scan(&leases); err != nil || leases != 1 {
			t.Fatal("socket retirement released UID", leases, err)
		}
	})
	for _, point := range []string{"checkpoint", "rename", "unlink"} {
		t.Run("interruption-"+point, func(t *testing.T) {
			m, record, path, stage := nativeVideoSocketFixture(t)
			lost := errors.New("fixture interruption")
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
					match = current == "socket-removing" && sourceErr == nil
				}
				if match {
					fired = true
					return lost
				}
				return nil
			}
			got, err := m.retireVideoSocket(context.Background(), record, interrupted)
			if !fired || !errors.Is(err, lost) || got.Stage != "" {
				t.Fatal("interruption not retained", fired, got.Stage, err)
			}
			var current string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil || current != "socket-removing" {
				t.Fatal("lost checkpoint", current, err)
			}
			next, err := m.retireVideoSocket(context.Background(), record, guard)
			if err != nil || next.Stage != "socket-removed" {
				t.Fatal("interrupted removal did not reconcile", next.Stage, err)
			}
		})
	}
	for _, kind := range []string{"replacement", "collision", "foreign-entry", "symlink", "hardlink"} {
		t.Run("refuse-"+kind, func(t *testing.T) {
			m, record, path, stage := nativeVideoSocketFixture(t)
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
				nativeRetirementSocket(t, path, 200000, 64056)
			case "collision":
				if err := os.WriteFile(stage, []byte("preserve foreign stage"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign-entry":
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), "foreign"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(filepath.Dir(m.Channels), "target")
				if err := os.WriteFile(target, []byte("preserve target"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(filepath.Dir(path), "alias")); err != nil {
					t.Fatal(err)
				}
			}
			var before unix.Stat_t
			if unix.Lstat(path, &before) != nil {
				t.Fatal("fixture source absent")
			}
			got, err := m.retireVideoSocket(context.Background(), record, guard)
			if !errors.Is(err, ErrPolicy) || got.Stage != "" {
				t.Fatal("foreign resource admitted", got.Stage, err)
			}
			var after unix.Stat_t
			if unix.Lstat(path, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode {
				t.Fatal("foreign source changed")
			}
			var current string
			if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil || current != "prepared" {
				t.Fatal("failed admission advanced journal", current, err)
			}
			if kind == "collision" {
				contents, err := os.ReadFile(stage)
				if err != nil || string(contents) != "preserve foreign stage" {
					t.Fatal("collision overwritten", err)
				}
			}
		})
	}
	t.Run("hardlink-policy-refusal-and-restoration", func(t *testing.T) {
		m, record, path, _ := nativeVideoSocketFixture(t)
		file, err := os.OpenFile("/proc/sys/fs/protected_hardlinks", os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		var fs unix.Statfs_t
		if unix.Fstatfs(int(file.Fd()), &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
			t.Fatal("unqualified sysctl fixture")
		}
		original, err := io.ReadAll(io.LimitReader(file, 16))
		if err != nil || strings.TrimSpace(string(original)) != "1" {
			t.Fatal("hardlink policy initially unqualified", err)
		}
		write := func(value []byte) error {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			n, err := file.Write(value)
			if err == nil && n != len(value) {
				return io.ErrShortWrite
			}
			return err
		}
		restored := false
		defer func() {
			if !restored {
				if err := write(original); err != nil {
					t.Error("hardlink policy restoration failed", err)
				}
			}
		}()
		if err := write([]byte("0\n")); err != nil {
			t.Fatal(err)
		}
		got, err := m.retireVideoSocket(context.Background(), record, guard)
		if !errors.Is(err, ErrPolicy) || got.Stage != "" {
			t.Fatal("disabled hardlink policy admitted retirement", got.Stage, err)
		}
		if _, err := os.Lstat(path); err != nil {
			t.Fatal("refused policy changed socket", err)
		}
		var current string
		if err := m.Store.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, videoRetirementKey(record.Intent.Request.InstanceID)+".stage").Scan(&current); err != nil || current != "prepared" {
			t.Fatal("refused policy advanced journal", current, err)
		}
		if err := write(original); err != nil {
			t.Fatal(err)
		}
		if err := requireVideoRetirementHardlinks(context.Background()); err != nil {
			t.Fatal("hardlink policy did not restore", err)
		}
		restored = true
		if next, err := m.retireVideoSocket(context.Background(), record, guard); err != nil || next.Stage != "socket-removed" {
			t.Fatal("restored policy did not admit qualified socket", next.Stage, err)
		}
		t.Log("hardlink-disabled refusal and restoration passed")
	})
	if !t.Failed() {
		t.Log("reserved video socket filesystem component passed; injected stopped guard is not live KVM or UID-release qualification")
	}
}
