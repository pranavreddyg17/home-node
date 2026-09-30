package guestinit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	if os.Getenv("HOMENODE_GUEST_INIT_CAPABILITIES") == "1" {
		status, err := os.ReadFile("/proc/self/status")
		if err != nil {
			t.Fatal(err)
		}
		observed := map[string]uint64{}
		for _, line := range strings.Split(string(status), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			switch fields[0] {
			case "CapEff:", "CapPrm:", "CapBnd:", "CapInh:", "CapAmb:":
				value, err := strconv.ParseUint(fields[1], 16, 64)
				if err != nil {
					t.Fatal(err)
				}
				observed[fields[0]] = value
			case "NoNewPrivs:":
				if fields[1] != "1" {
					t.Fatal("new privileges not disabled")
				}
				observed[fields[0]] = 1
			}
		}
		for _, name := range []string{"CapEff:", "CapPrm:", "CapBnd:"} {
			if observed[name] != 5 {
				t.Fatal("unexpected service capability set")
			}
		}
		for _, name := range []string{"CapInh:", "CapAmb:"} {
			if value, ok := observed[name]; !ok || value != 0 {
				t.Fatal("unexpected inherited capabilities")
			}
		}
		if observed["NoNewPrivs:"] != 1 {
			t.Fatal("missing privilege restriction evidence")
		}
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
	t.Run("resume owned empty creation", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, intentName), []byte(intentBytes), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(parent, "objects"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(parent); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(parent); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(parent, "objects"))
		if err != nil {
			t.Fatal(err)
		}
		owner := info.Sys().(*syscall.Stat_t)
		if owner.Uid != 900 || owner.Gid != 900 {
			t.Fatal("owned creation not completed")
		}
	})
	t.Run("preserve nonempty owned creation", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, intentName), []byte(intentBytes), 0600); err != nil {
			t.Fatal(err)
		}
		objects := filepath.Join(parent, "objects")
		if err := os.Mkdir(objects, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(objects, "preserved"), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		if Prepare(parent) == nil {
			t.Fatal("nonempty root directory adopted")
		}
		info, err := os.Stat(objects)
		if err != nil {
			t.Fatal(err)
		}
		if info.Sys().(*syscall.Stat_t).Uid != 0 {
			t.Fatal("rejected directory ownership changed")
		}
	})
	t.Run("preserve corrupt intent", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, intentName), []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
		if Prepare(parent) == nil {
			t.Fatal("corrupt intent accepted")
		}
		if _, err := os.Stat(filepath.Join(parent, "objects")); !os.IsNotExist(err) {
			t.Fatal("corrupt intent mutated storage")
		}
	})

	t.Run("interrupted staging is preserved and retry succeeds", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		pending := filepath.Join(parent, ".homenode-init-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pending")
		if err := os.WriteFile(pending, []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(parent); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(pending)
		if err != nil || string(data) != "partial" {
			t.Fatal("interrupted staging changed")
		}
		intent, err := os.ReadFile(filepath.Join(parent, intentName))
		if err != nil || string(intent) != intentBytes {
			t.Fatal("active intent not completely published")
		}
	})
	t.Run("publication never overwrites existing intent", func(t *testing.T) {
		parent := t.TempDir()
		root, err := os.OpenRoot(parent)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := os.WriteFile(filepath.Join(parent, intentName), []byte("foreign"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, "pending"), []byte(intentBytes), 0600); err != nil {
			t.Fatal(err)
		}
		if publishIntent(root, "pending") == nil {
			t.Fatal("occupied intent replaced")
		}
		data, err := os.ReadFile(filepath.Join(parent, intentName))
		if err != nil || string(data) != "foreign" {
			t.Fatal("existing intent changed")
		}
		if _, err := os.Stat(filepath.Join(parent, "pending")); err != nil {
			t.Fatal("failed publication removed staging")
		}
	})
	t.Run("bounded interrupted staging", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
		for n := range 64 {
			name := fmt.Sprintf(".homenode-init-%032x.pending", n)
			if err := os.WriteFile(filepath.Join(parent, name), []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if Prepare(parent) == nil {
			t.Fatal("unbounded staging accepted")
		}
		if _, err := os.Stat(filepath.Join(parent, intentName)); !os.IsNotExist(err) {
			t.Fatal("staging refusal published intent")
		}
	})

	for _, kind := range []string{"symlink intent", "hardlink intent", "fifo intent", "public intent", "foreign intent", "special mode intent"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(parent, intentName)
			switch kind {
			case "symlink intent":
				target := filepath.Join(parent, "other")
				if err := os.WriteFile(target, []byte(intentBytes), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, marker); err != nil {
					t.Fatal(err)
				}
			case "fifo intent":
				if err := syscall.Mkfifo(marker, 0600); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(marker, []byte(intentBytes), 0600); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "hardlink intent":
					if err := os.Link(marker, filepath.Join(parent, "alias")); err != nil {
						t.Fatal(err)
					}
				case "public intent":
					if err := os.Chmod(marker, 0644); err != nil {
						t.Fatal(err)
					}
				case "foreign intent":
					if err := os.Chown(marker, 901, 901); err != nil {
						t.Fatal(err)
					}
				case "special mode intent":
					if err := os.Chmod(marker, 0600|os.ModeSetuid); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(marker)
			if err != nil {
				t.Fatal(err)
			}
			if Prepare(parent) == nil {
				t.Fatal("unsafe intent accepted")
			}
			after, err := os.Lstat(marker)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("rejected intent changed")
			}
			if _, err := os.Stat(filepath.Join(parent, "objects")); !os.IsNotExist(err) {
				t.Fatal("rejected intent created objects")
			}
		})
	}

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
					if err := os.Chmod(path, 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.Chown(path, 900, 900); err != nil {
						t.Fatal(err)
					}
				}
			}
			if kind != "symlink" {
				t.Cleanup(func() {
					if err := os.Chown(path, 0, 0); err != nil {
						t.Error(err)
					}
				})
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
