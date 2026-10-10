//go:build linux

package supervisor

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// This uses the shipped transfer binary, a real Manager HTTP handler, and
// native peer credentials. It does not qualify installed systemd or user TLS.
func startNativeFilesTransfer(t *testing.T, ctx context.Context, base, script, instanceID string, m *Manager, gid int) (func(), func()) {
	t.Helper()
	runtimeSocket := filepath.Join(base, "transfer-supervisor.sock")
	listener, err := net.Listen("unix", runtimeSocket)
	if err != nil {
		t.Fatal(err)
	}
	retained := false
	defer func() {
		if !retained {
			_ = listener.Close()
		}
	}()
	if err := os.Chown(runtimeSocket, 0, gid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimeSocket, 0660); err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: m.Handler(), ConnContext: PeerContext,
		ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 95 * time.Second, MaxHeaderBytes: 8192}
	defer func() {
		if !retained {
			_ = server.Close()
		}
	}()
	go func() { _ = server.Serve(listener) }()
	exercise, stop := startNativeFilesTransferAt(t, ctx, base, script, instanceID, runtimeSocket, m.Channels, gid)
	retained = true
	return exercise, func() {
		stop()
		_ = server.Close()
		_ = listener.Close()
	}
}

// Reuse the shipped transfer/client path against either the real executable's
// socket or the direct Manager fixture. Peer authorization stays server-side.
func startNativeFilesTransferAt(t *testing.T, ctx context.Context, base, script, instanceID, runtimeSocket, channels string, gid int) (func(), func()) {
	t.Helper()
	binary := "/usr/lib/homenode-fixtures/homenode-transfer"
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0755 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		t.Fatal("native transfer fixture binary unavailable", err)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != 0 || metadata.Nlink != 1 {
		t.Fatal("unprotected native transfer fixture binary")
	}
	retained := false
	directory := filepath.Join(base, "transfer-runtime")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 2, gid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0750); err != nil {
		t.Fatal(err)
	}
	transferSocket := filepath.Join(directory, "transfer.sock")
	childCtx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(childCtx, binary, "-socket", transferSocket, "-supervisor", runtimeSocket,
		"-channels", channels, "-controller-uid", "1", "-access-gid", strconv.Itoa(gid), "-policy-generation", "1")
	command.Dir = base
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	var diagnostics boundedOutput
	command.Stdout, command.Stderr = &diagnostics, &diagnostics
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 2, Gid: uint32(gid), Groups: []uint32{}}}
	command.WaitDelay = 3 * time.Second
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal("native transfer launch", err)
	}
	stop := func() {
		cancel()
		_ = command.Wait() // Cancellation kills/reaps this direct fixture child.
		if output := diagnostics.String(); output != "" {
			t.Log("native transfer process diagnostics", output)
		}
	}
	defer func() {
		if !retained {
			stop()
		}
	}()
	client := filepath.Join(filepath.Dir(script), "manager_transfer.py")
	exercise := func() {
		for _, scenario := range []struct {
			uid  uint32
			mode string
		}{{3, "deny"}, {2, "runtime-deny"}, {1, "allow"}} {
			endpoint := transferSocket
			if scenario.mode == "runtime-deny" {
				endpoint = runtimeSocket
			}
			clientCommand := exec.CommandContext(ctx, "/usr/bin/python3", "-B", client, endpoint, instanceID, scenario.mode)
			clientCommand.Dir = filepath.Dir(client)
			clientCommand.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOMENODE_FILES_MANAGER_INTEGRATION=1"}
			clientCommand.WaitDelay = time.Second
			clientCommand.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: scenario.uid, Gid: uint32(gid), Groups: []uint32{}}}
			output, err := clientCommand.CombinedOutput()
			if err != nil || string(output) != "development transfer "+scenario.mode+" passed\n" {
				t.Fatalf("native transfer %s: %v: %s", scenario.mode, err, output)
			}
		}
		t.Log("actual unprivileged transfer binary round trip, foreign controller UID refusal and transfer UID runtime-stop refusal passed")
	}
	retained = true
	return exercise, stop
}
