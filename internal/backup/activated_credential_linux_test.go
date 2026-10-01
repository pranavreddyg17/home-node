//go:build linux

package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/socketactivation"
)

func TestNativeActivatedCredentialHandoff(t *testing.T) {
	role, path := os.Getenv("HOMENODE_CREDENTIAL_ROLE"), os.Getenv("HOMENODE_CREDENTIAL_PATH")
	if role != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if role == "worker" {
			if os.Geteuid() != 1003 {
				t.Fatal("wrong worker UID")
			}
			if connection, err := net.Dial("unixpacket", path); err == nil {
				connection.Close()
				t.Fatal("worker could dial controller-only socket")
			}
			os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
			listener, err := socketactivation.TakePrivatePacketListener("homenode-backup-credential", path, 1001)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			listener.(*net.UnixListener).SetDeadline(time.Now().Add(8 * time.Second))
			connection, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			job, credential, err := ReceiveCredentialDispatch(ctx, connection.(*net.UnixConn), 1001)
			if err != nil {
				t.Fatal(err)
			}
			data, err := ReadRepositoryPassword(ctx, credential)
			if err != nil || !bytes.Equal(data, []byte("fixture-cross-uid-secret")) {
				credential.Close()
				clear(data)
				t.Fatal("credential handoff invalid", err)
			}
			clear(data)
			if err = credential.Close(); err != nil {
				t.Fatal(err)
			}
			if os.Getenv("HOMENODE_CREDENTIAL_MISMATCH") == "1" {
				job.JobID = strings.Repeat("b", 24)
			}
			if err = sendWorkerCompletion(ctx, connection.(*net.UnixConn), job); err != nil {
				t.Fatal(err)
			}
			return
		}
		if role != "controller" || os.Geteuid() != 1001 {
			t.Fatal("wrong controller role")
		}
		connection, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Net: "unixpacket", Name: path})
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		id := strings.Repeat("a", 24)
		job := Dispatch{Version: 1, JobID: id, DeviceID: id, ManagementToken: id, RuntimeToken: id, Release: "0.1.0", CatalogVersion: 1}
		credential, err := CreateRepositoryPassword([]byte("fixture-cross-uid-secret"))
		if err != nil {
			t.Fatal(err)
		}
		defer credential.Close()
		if err = SendCredentialDispatch(ctx, connection, 1003, job, credential); err == nil {
			t.Fatal("ordinary sender trusted root creator")
		}
		err = SendActivatedCredentialDispatchAndWait(ctx, connection, job, credential)
		if os.Getenv("HOMENODE_CREDENTIAL_MISMATCH") == "1" {
			if !errors.Is(err, ErrManifest) {
				t.Fatal("foreign job completion accepted", err)
			}
		} else if err != nil {
			t.Fatal("completion missing", err)
		}
		if _, err = credential.Stat(); err != nil {
			t.Fatal("sender credential closed", err)
		}
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_CREDENTIAL_INTEGRATION") != "1" {
		t.Skip("opt-in disposable root fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-credential-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err = os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	binary := filepath.Join(directory, "handoff.test")
	output, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	path = filepath.Join(directory, "credential.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chown(path, 0, 1001); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	inherited, err := listener.File()
	if err != nil {
		t.Fatal(err)
	}
	defer inherited.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	worker := exec.CommandContext(ctx, binary, "-test.run=^TestNativeActivatedCredentialHandoff$", "-test.v")
	worker.Env = []string{"HOMENODE_CREDENTIAL_ROLE=worker", "HOMENODE_CREDENTIAL_PATH=" + path, "LISTEN_FDS=1", "LISTEN_FDNAMES=homenode-backup-credential"}
	worker.Env = append(worker.Env, "HOMENODE_CREDENTIAL_MISMATCH="+os.Getenv("HOMENODE_CREDENTIAL_MISMATCH"))
	worker.ExtraFiles = []*os.File{inherited}
	worker.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1003, Gid: 1003, Groups: []uint32{}}}
	var workerOutput bytes.Buffer
	worker.Stdout, worker.Stderr = &workerOutput, &workerOutput
	if err = worker.Start(); err != nil {
		t.Fatal(err)
	}
	controller := exec.CommandContext(ctx, binary, "-test.run=^TestNativeActivatedCredentialHandoff$", "-test.v")
	controller.Env = []string{"HOMENODE_CREDENTIAL_ROLE=controller", "HOMENODE_CREDENTIAL_PATH=" + path}
	controller.Env = append(controller.Env, "HOMENODE_CREDENTIAL_MISMATCH="+os.Getenv("HOMENODE_CREDENTIAL_MISMATCH"))
	controller.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1001, Gid: 1001, Groups: []uint32{}}}
	var controllerOutput bytes.Buffer
	controller.Stdout, controller.Stderr = &controllerOutput, &controllerOutput
	controllerErr := controller.Run()
	if controllerErr != nil {
		cancel()
	}
	workerErr := worker.Wait()
	if controllerErr != nil || workerErr != nil {
		t.Fatal("activated credential handoff failed", controllerErr, workerErr, controllerOutput.String(), workerOutput.String())
	}
}

func TestNativeActivatedCredentialCompletionMismatch(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_CREDENTIAL_INTEGRATION") != "1" {
		t.Skip("opt-in disposable root fixture")
	}
	t.Setenv("HOMENODE_CREDENTIAL_MISMATCH", "1")
	TestNativeActivatedCredentialHandoff(t)
}
