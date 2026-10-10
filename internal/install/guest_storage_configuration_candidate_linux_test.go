//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRootGuestStorageConfigurationCandidateRefusesUnsafeSources(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	for _, kind := range []string{"oversized", "fifo", "symlink", "hardlink", "writable", "foreign-owner", "duplicate-record"} {
		t.Run(kind, func(t *testing.T) {
			host, journalDir := roots(t)
			directory := filepath.Join(host, "etc", "homenode")
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "runtime-policy.json")
			data := []byte("owned configuration\n")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			installed := journal{Items: []record{{Path: "etc/homenode/runtime-policy.json", Mode: 0600, SHA256: digest(data), State: "created"}}}
			switch kind {
			case "oversized":
				data = make([]byte, 16385)
				installed.Items[0].SHA256 = digest(data)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(path, 0620); err != nil {
					t.Fatal(err)
				}
			case "foreign-owner":
				if err := os.Chown(path, 65534, 0); err != nil {
					t.Fatal(err)
				}
			case "duplicate-record":
				installed.Items = append(installed.Items, installed.Items[0])
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			engine := openEngine(t, host, journalDir)
			defer engine.Close()
			candidate, err := engine.readGuestStorageConfigurationCandidate(context.Background(), installed, "runtime-policy.json", 0600)
			if err == nil || candidate != nil {
				t.Fatal("unsafe source returned trusted bytes", kind, err)
			}
			if kind != "symlink" && !errors.Is(err, ErrConflict) {
				t.Fatal("unexpected refusal", err)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
				t.Fatal("rejected source evidence changed", err)
			}
		})
	}
}
