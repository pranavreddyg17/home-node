//go:build linux

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

// Join the native workload executables to the actual confined HTTPS services.
// The controller must allocate its own instance in a fresh supervisor journal.
func nativeFilesHTTPSWorkflow(t *testing.T, ctx context.Context, directory, script, socket string, policy Policy, gid int, safeCleanup *bool, restartSupervisor func() error) string {
	t.Helper()
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_FILES_HTTPS_INTEGRATION") != "1" || os.Getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1" || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" {
		t.Fatal("explicit disposable HTTPS Files integration required")
	}
	payloadBytes := os.Getenv("HOMENODE_FILES_HTTPS_BYTES")
	if payloadBytes == "" {
		payloadBytes = "32768"
	}
	if payloadBytes != "32768" && payloadBytes != "1073741824" {
		t.Fatal("unsupported native HTTPS payload size")
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
		if readErr != nil || len(ids) > policy.MaxInstances {
			*safeCleanup = false
			t.Error("unexpected joined workflow allocations", len(ids), readErr)
			return
		}
		backend := LinuxBackend{DataRoot: filepath.Join(directory, "volumes"), TransferGID: gid}
		for _, id := range ids {
			if t.Failed() {
				// Observe the failed guest before cleanup removes its process and
				// channel. These are disposable fixture domains, never owner VMs.
				name := "homenode-" + id
				output, err := nativeFixtureCommand(cleanup, "/usr/bin/virsh", "--connect", "qemu:///system", "domstate", name, "--reason")
				t.Log("joined failed guest state", output, err)
				nativeDomainDiagnostics(t, Domain{ID: id, ChannelPath: filepath.Join(directory, "channels", id, "adapter.sock")})
			}
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
	defer func() {
		if stop != nil {
			stop()
		}
	}()
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
	restartSocket := filepath.Join(directory, "restart.sock")
	listener, err := net.Listen("unix", restartSocket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restartSocket, 0600); err != nil {
		listener.Close()
		t.Fatal(err)
	}
	restarts := make(chan chan error)
	server := &http.Server{ConnContext: PeerContext, ReadHeaderTimeout: time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 90 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := RequestPeerUID(r)
		if !ok || uid != 0 || r.Method != "POST" || r.URL.Path != "/restart" {
			http.Error(w, "denied", 403)
			return
		}
		answer := make(chan error, 1)
		select {
		case restarts <- answer:
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		}
		select {
		case err := <-answer:
			if err != nil {
				http.Error(w, "restart failed", 500)
				return
			}
			w.Write([]byte("restarted"))
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	binaries := "/usr/lib/homenode-fixtures"
	pythonCtx, cancelPython := context.WithCancel(ctx)
	defer cancelPython()
	command := exec.CommandContext(pythonCtx, "/usr/bin/python3", "-B", filepath.Join(root, "packaging/inspection/gateway_startup_fixture.py"),
		"--gateway-binary", filepath.Join(binaries, "homenode-gateway"), "--controller-binary", filepath.Join(binaries, "homenode"),
		"--identity-client-binary", filepath.Join(binaries, "https-identity.test"), "--web-dist", filepath.Join(root, "web/dist"),
		"--source-service", filepath.Join(root, "packaging/systemd/homenode-gateway.service"),
		"--controller-source-service", filepath.Join(root, "packaging/systemd/homenode-control.service"),
		"--supervisor-socket", socket, "--transfer-socket", transferSocket, "--files-bytes", payloadBytes, "--restart-socket", restartSocket)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "HOMENODE_GATEWAY_STARTUP_FIXTURE=1", "HOMENODE_FILES_HTTPS_INTEGRATION=1", "HOMENODE_FILES_MANAGER_INTEGRATION=1", "HOMENODE_KVM_DOMAIN_INTEGRATION=1"}
	if os.Getenv("HOMENODE_FILES_BROWSER_INTEGRATION") == "1" {
		if payloadBytes != "32768" {
			t.Fatal("native browser qualification requires the bounded protocol profile")
		}
		browserBytes := os.Getenv("HOMENODE_FILES_BROWSER_BYTES")
		if browserBytes == "" {
			browserBytes = "32768"
		}
		if browserBytes != "32768" && browserBytes != "1073741824" {
			t.Fatal("unsupported native browser payload size")
		}
		command.Args = append(command.Args, "--browser-bytes", browserBytes)
		command.Args = append(command.Args, "--browser-node", filepath.Join(binaries, "node"), "--browser-executable", "/opt/google/chrome/chrome", "--browser-client", filepath.Join(root, "web/native/files-https.mjs"), "--browser-modules", filepath.Join(root, "web/node_modules"))
		command.Env = append(command.Env, "HOMENODE_FILES_BROWSER_INTEGRATION=1")
	}
	if os.Getenv("HOMENODE_VIDEO_HTTPS_INTEGRATION") == "1" {
		command.Args = append(command.Args, "--video")
		command.Env = append(command.Env, "HOMENODE_VIDEO_HTTPS_INTEGRATION=1")
	}
	command.WaitDelay = 5 * time.Second
	var output boundedOutput
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	waited := false
	defer func() {
		if !waited {
			cancelPython()
			<-done
		}
	}()
	restartCount := 0
	var runErr error
	running := true
	for running {
		select {
		case runErr = <-done:
			waited = true
			running = false
		case answer := <-restarts:
			if restartCount != 0 {
				answer <- errors.New("duplicate restart")
				continue
			}
			restartCount++
			*safeCleanup = false
			oldPID, err := nativeFixtureListenerPID(transferSocket, policy.TransferUID)
			if err == nil {
				stop()
				stop = nil
				err = os.Remove(filepath.Dir(transferSocket)) // Empty owned runtime directory only.
			}
			if err == nil {
				err = restartSupervisor()
			}
			if err == nil {
				_, stop = startNativeFilesTransferAt(t, ctx, directory, script, "", socket, filepath.Join(directory, "channels"), gid, policy.ControllerUID, policy.TransferUID)
				until := time.Now().Add(10 * time.Second)
				for time.Now().Before(until) {
					var pid int
					pid, err = nativeFixtureListenerPID(transferSocket, policy.TransferUID)
					if err == nil && pid != oldPID {
						t.Logf("actual transfer restart pid %d -> %d", oldPID, pid)
						break
					}
					if err == nil {
						err = errors.New("transfer PID did not change")
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
			if err == nil {
				*safeCleanup = true
			} else {
				t.Log("native service restart refused", err)
			}
			answer <- err
		}
	}
	if runErr != nil || output.tooLarge || restartCount != 1 {
		*safeCleanup = false // A controller worker may still have effects in flight.
		t.Fatal("actual HTTPS Files workflow failed", runErr, restartCount, output.String())
	}
	t.Log(output.String())
	store, err := state.Open(journal)
	if err != nil {
		*safeCleanup = false
		t.Fatal(err)
	}
	defer store.Close()
	var id, status string
	if err := store.DB.QueryRowContext(ctx, "SELECT id,state FROM runtime_instances WHERE workload='files'").Scan(&id, &status); err != nil || !guestproto.ValidID(id) || status != "stopped" {
		*safeCleanup = false
		t.Fatal("HTTPS controller allocation did not finish stopped", status, err)
	}
	return id
}

func nativeFixtureListenerPID(socket string, uid uint32) (int, error) {
	connection, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	control, err := connection.(*net.UnixConn).SyscallConn()
	if err != nil {
		return 0, err
	}
	var credentials *unix.Ucred
	var observeErr error
	err = control.Control(func(fd uintptr) {
		credentials, observeErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil || observeErr != nil || credentials == nil || credentials.Uid != uid || credentials.Pid <= 0 {
		return 0, fmt.Errorf("native listener identity unavailable: %v %v", err, observeErr)
	}
	return int(credentials.Pid), nil
}
