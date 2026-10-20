//go:build linux

package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// Join the native workload executables to the actual confined HTTPS services.
// The controller must allocate its own instance in a fresh supervisor journal.
func nativeFilesHTTPSWorkflow(t *testing.T, ctx context.Context, directory, script, socket string, policy Policy, gid int, safeCleanup *bool) string {
	t.Helper()
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_FILES_HTTPS_INTEGRATION") != "1" || os.Getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1" || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" {
		t.Fatal("explicit disposable HTTPS Files integration required")
	}
	journal := filepath.Join(directory, "journal")
	// This also runs when an HTTP failure leaves allocation/effects uncertain.
	// Never discard fixture storage without observing every allocated VM exit.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		store, err := state.Open(journal)
		if err != nil {
			*safeCleanup = false
			t.Error("cannot inspect joined workflow allocations", err)
			return
		}
		defer store.Close()
		rows, err := store.DB.QueryContext(cleanup, "SELECT id FROM runtime_instances LIMIT 3")
		if err != nil {
			*safeCleanup = false
			t.Error(err)
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil || !guestproto.ValidID(id) {
				*safeCleanup = false
				t.Error("invalid joined workflow allocation", err)
				rows.Close()
				return
			}
			ids = append(ids, id)
		}
		readErr := rows.Err()
		rows.Close()
		if readErr != nil || len(ids) > 1 {
			*safeCleanup = false
			t.Error("unexpected joined workflow allocations", len(ids), readErr)
			return
		}
		backend := LinuxBackend{DataRoot: filepath.Join(directory, "volumes"), TransferGID: gid}
		for _, id := range ids {
			if err := backend.Stop(cleanup, id); err != nil {
				*safeCleanup = false
				t.Error("joined workflow guest cleanup", err)
				continue
			}
			if running, err := backend.Running(cleanup, id); err != nil || running {
				*safeCleanup = false
				t.Error("joined workflow guest exit unverified", running, err)
			}
		}
	}()
	_, stop := startNativeFilesTransferAt(t, ctx, directory, script, "", socket, filepath.Join(directory, "channels"), gid, policy.ControllerUID, policy.TransferUID)
	defer stop()
	transferSocket := filepath.Join(directory, "transfer-runtime", "transfer.sock")
	deadline := time.Now().Add(10 * time.Second)
	for {
		info, err := os.Lstat(transferSocket)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			t.Fatal("joined transfer socket did not appear", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binaries := "/usr/lib/homenode-fixtures"
	command := exec.CommandContext(ctx, "/usr/bin/python3", "-B", filepath.Join(root, "packaging/inspection/gateway_startup_fixture.py"),
		"--gateway-binary", filepath.Join(binaries, "homenode-gateway"), "--controller-binary", filepath.Join(binaries, "homenode"),
		"--identity-client-binary", filepath.Join(binaries, "https-identity.test"), "--web-dist", filepath.Join(root, "web/dist"),
		"--source-service", filepath.Join(root, "packaging/systemd/homenode-gateway.service"),
		"--controller-source-service", filepath.Join(root, "packaging/systemd/homenode-control.service"),
		"--supervisor-socket", socket, "--transfer-socket", transferSocket)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "HOMENODE_GATEWAY_STARTUP_FIXTURE=1", "HOMENODE_FILES_HTTPS_INTEGRATION=1", "HOMENODE_FILES_MANAGER_INTEGRATION=1", "HOMENODE_KVM_DOMAIN_INTEGRATION=1"}
	command.WaitDelay = 5 * time.Second
	var output boundedOutput
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil || output.tooLarge {
		*safeCleanup = false // A controller worker may still have effects in flight.
		t.Fatal("actual HTTPS Files workflow failed", err, output.String())
	}
	t.Log(output.String())
	store, err := state.Open(journal)
	if err != nil {
		*safeCleanup = false
		t.Fatal(err)
	}
	defer store.Close()
	var id, status string
	if err := store.DB.QueryRowContext(ctx, "SELECT id,state FROM runtime_instances").Scan(&id, &status); err != nil || !guestproto.ValidID(id) || status != "stopped" {
		*safeCleanup = false
		t.Fatal("HTTPS controller allocation did not finish stopped", status, err)
	}
	return id
}
