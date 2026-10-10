//go:build linux

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Uses the built executable rather than calling a startup helper. This proves
// refusal ordering; it does not establish installed service or VM acceptance.
func TestRootActualSupervisorLiveEndpointRefusal(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_SUPERVISOR_ENTRY_INTEGRATION") != "1" {
		t.Skip("disposable Linux root executable fixture only")
	}
	binary := "/usr/lib/homenode-fixtures/homenode-supervisor"
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0755 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		t.Fatal("fixture executable unavailable", err)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != 0 || metadata.Nlink != 1 {
		t.Fatal("unprotected fixture executable")
	}
	base := t.TempDir()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := json.Marshal(supervisor.Policy{Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: 1, TransferUID: 2})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"policy.json": policy, "catalog.pub": []byte(hex.EncodeToString(pub)), "catalog-floor": []byte("1\n"), "catalog.json": signedStartupCatalog(t, key, 1, time.Now()),
	} {
		if err := os.WriteFile(filepath.Join(base, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	socket := filepath.Join(base, "supervisor.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: socket})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chown(socket, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(socket, 0660); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary,
		"--policy", filepath.Join(base, "policy.json"), "--publisher-key", filepath.Join(base, "catalog.pub"),
		"--catalog-floor", filepath.Join(base, "catalog-floor"), "--catalog", filepath.Join(base, "catalog.json"),
		"--state-dir", filepath.Join(base, "journal"), "--images", filepath.Join(base, "images"),
		"--volumes", filepath.Join(base, "volumes"), "--channels", filepath.Join(base, "channels"),
		"--socket", socket, "--access-gid", "1", "--transfer-gid", "2")
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	command.WaitDelay = time.Second
	output, err := command.CombinedOutput()
	if ctx.Err() != nil || err == nil || !strings.Contains(string(output), "live endpoint preserved") {
		t.Fatalf("startup did not refuse live endpoint: %v %s", err, output)
	}
	after, err := os.Lstat(socket)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("live endpoint replaced", err)
	}
	for _, name := range []string{"journal", "images", "volumes", "channels"} {
		if _, err := os.Lstat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Fatal("startup mutated runtime before endpoint admission", name, err)
		}
	}
	// Refusal must also release its own parent exclusion on process exit.
	parents, err := lockEndpointParents([]string{socket})
	if err != nil {
		t.Fatal("failed startup retained exclusion", err)
	}
	for _, parent := range parents {
		parent.Close()
	}
}
