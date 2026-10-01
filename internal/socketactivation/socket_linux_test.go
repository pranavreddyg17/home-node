//go:build linux

package socketactivation

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestNativeInheritedPrivateListener(t *testing.T) {
	network, descriptorName, packetFlag := "unix", "homenode-app-maintenance", "0"
	packetMode := os.Getenv("HOMENODE_ACTIVATION_PACKET") == "1"
	if packetMode {
		network, descriptorName, packetFlag = "unixpacket", "homenode-backup-credential", "1"
	}
	if os.Getenv("HOMENODE_ACTIVATION_CHILD") == "1" {
		if os.Geteuid() != 1001 {
			t.Fatal("unexpected controller identity")
		}
		path := os.Getenv("HOMENODE_ACTIVATION_PATH")
		if connection, err := net.Dial(network, path); err == nil {
			connection.Close()
			t.Fatal("controller unexpectedly has backup socket access")
		}
		if os.Getenv("HOMENODE_ACTIVATION_SYSTEMD") == "1" {
			if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
				t.Fatal("systemd activation PID missing")
			}
		} else {
			os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
		}
		var listener net.Listener
		var err error
		if packetMode {
			listener, err = TakePrivatePacketListener(descriptorName, path, 1003)
		} else {
			listener, err = TakePrivateListener(descriptorName, path, 1003)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		if os.Getenv("LISTEN_FDS") != "" || os.Getenv("LISTEN_PID") != "" || os.Getenv("LISTEN_FDNAMES") != "" {
			t.Fatal("activation environment leaked")
		}
		listener.(*net.UnixListener).SetDeadline(time.Now().Add(10 * time.Second))
		connection, err := listener.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if _, err = connection.Write([]byte("controller-1001")); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_ACTIVATION_INTEGRATION") != "1" {
		t.Skip("opt-in disposable root fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-activation-")
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
	binary := filepath.Join(directory, "activation.test")
	target, err := os.OpenFile(binary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	path := filepath.Join(directory, "apps.sock")
	listener, err := net.ListenUnix(network, &net.UnixAddr{Net: network, Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chown(path, 0, 1003); err != nil {
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
	command := exec.CommandContext(ctx, binary, "-test.run=^TestNativeInheritedPrivateListener$", "-test.v")
	command.ExtraFiles = []*os.File{inherited}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1001, Gid: 1001, Groups: []uint32{}}}
	command.Env = []string{"HOMENODE_ACTIVATION_CHILD=1", "HOMENODE_ACTIVATION_PATH=" + path, "LISTEN_FDS=1", "LISTEN_FDNAMES=" + descriptorName, "HOMENODE_ACTIVATION_PACKET=" + packetFlag}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, path)
	if err != nil {
		cancel()
		command.Wait()
		t.Fatal(err)
	}
	connection.SetDeadline(time.Now().Add(10 * time.Second))
	uid, peerErr := supervisor.PeerUID(connection.(*net.UnixConn))
	data, readErr := io.ReadAll(connection)
	connection.Close()
	waitErr := command.Wait()
	if peerErr != nil || uid != 0 || readErr != nil || string(data) != "controller-1001" || waitErr != nil {
		t.Fatal("inherited listener identity", uid, peerErr, string(data), readErr, waitErr, output.String())
	}
}

func TestNativeInheritedPrivatePacketListener(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_ACTIVATION_INTEGRATION") != "1" {
		t.Skip("opt-in disposable root fixture")
	}
	t.Setenv("HOMENODE_ACTIVATION_PACKET", "1")
	TestNativeInheritedPrivateListener(t)
}
