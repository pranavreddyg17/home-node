//go:build linux

package supervisor

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// The parent native fixture supplies a running QEMU PID; this child only observes it.
func TestNativeSupervisorMemoryObservation(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_SUPERVISOR_SOURCE_FIXTURE") != "1" || os.Getenv("HOMENODE_SUPERVISOR_MEMORY_PID") == "" {
		t.Skip("explicit source-protected native memory fixture")
	}
	pid, err := strconv.Atoi(os.Getenv("HOMENODE_SUPERVISOR_MEMORY_PID"))
	if err != nil || pid <= 0 {
		t.Fatal("invalid process fixture", err)
	}
	id := os.Getenv("HOMENODE_SUPERVISOR_MEMORY_ID")
	if !guestproto.ValidID(id) {
		t.Fatal("invalid domain fixture")
	}
	maximum, err := strconv.ParseInt(os.Getenv("HOMENODE_SUPERVISOR_MEMORY_MAX"), 10, 64)
	if err != nil || maximum <= 1 {
		t.Fatal("invalid memory fixture", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := observeGuestMemoryProcess(ctx, pid, id, maximum); err != nil {
		t.Fatal("source-protected native memory observation", err)
	}
	if err := observeGuestMemoryProcess(ctx, pid, id, maximum-1); err == nil {
		t.Fatal("excessive memory admitted")
	}
	if err := observeGuestMemoryProcess(ctx, pid, state.Random(), maximum); err == nil {
		t.Fatal("foreign domain admitted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err := observeGuestMemoryProcess(cancelled, pid, id, maximum); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
