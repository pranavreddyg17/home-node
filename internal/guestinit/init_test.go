package guestinit

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestUnprivilegedInitializerRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unprivileged fixture only")
	}
	root := t.TempDir()
	if Prepare(root) == nil {
		t.Fatal("unprivileged initialization admitted")
	}
	if _, err := os.Stat(filepath.Join(root, "objects")); !os.IsNotExist(err) {
		t.Fatal("unprivileged initialization changed data")
	}
}

func TestNativeGuestObjectInitialization(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_INIT_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	t.Run("create and reopen", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := Prepare(parent); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(filepath.Join(parent, "objects"))
		if err != nil {
			t.Fatal(err)
		}
		owner := info.Sys().(*syscall.Stat_t)
		if owner.Uid != 900 || owner.Gid != 900 || info.Mode().Perm() != 0700 {
			t.Fatal("incorrect private guest identity")
		}
	})
	for _, kind := range []string{"foreign", "wide", "symlink", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "objects")
			if kind == "symlink" {
				if err := os.Symlink(t.TempDir(), path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				marker := filepath.Join(path, "preserved")
				if err := os.WriteFile(marker, []byte("existing data"), 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "foreign" {
					if err := os.Chown(path, 901, 901); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "wide" {
					if err := os.Chown(path, 900, 900); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, 0755); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := Prepare(parent); err == nil {
				t.Fatal("unsafe object root adopted")
			}
			after, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			a, b := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() || a.Uid != b.Uid || a.Gid != b.Gid {
				t.Fatal("existing directory changed")
			}
			if kind != "symlink" {
				data, err := os.ReadFile(filepath.Join(path, "preserved"))
				if err != nil || string(data) != "existing data" {
					t.Fatal("existing bytes changed")
				}
			}
		})
	}
}
