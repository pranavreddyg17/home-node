//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestMemoryDomainRefusesSyntheticFilesystem(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("A", 26)
	relative := `/homenode.slice/machine-qemu\x2d1\x2dhomenode\x2d` + id + ".scope/libvirt/emulator"
	// Plausible synthetic control files must never qualify a kernel bound.
	target := filepath.Join(directory, strings.TrimPrefix(relative, "/"))
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"cgroup.type": "domain\n", "memory.max": "1024\n"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := observeGuestMemoryDomainAt(context.Background(), directory, relative, id, 1024); err == nil {
		t.Fatal("ordinary filesystem qualified as cgroup2")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := observeGuestMemoryDomainAt(cancelled, directory, relative, id, 1024); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	if err := observeGuestMemoryDomainAt(context.Background(), alias, relative, id, 1024); err == nil {
		t.Fatal("symlink root admitted")
	}
	for _, maximum := range []int64{0, -1} {
		if err := observeGuestMemoryDomainAt(context.Background(), directory, relative, id, maximum); err == nil {
			t.Fatal("invalid bound admitted")
		}
	}
}
