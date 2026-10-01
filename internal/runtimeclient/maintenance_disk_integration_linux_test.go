//go:build linux

package runtimeclient

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestNativeBackupDiskClient(t *testing.T) {
	if os.Getenv("HOMENODE_DISK_CLIENT_CHILD") == "1" {
		if os.Geteuid() != 1003 {
			t.Fatal("unexpected child identity")
		}
		if file, err := os.Open(os.Getenv("HOMENODE_DISK_SOURCE")); err == nil {
			file.Close()
			t.Fatal("backup user opened root path")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var received *os.File
		err := NewDiskClient(os.Getenv("HOMENODE_DISK_SOCKET")).WithMaintenanceDisk(ctx, os.Getenv("HOMENODE_DISK_TOKEN"), os.Getenv("HOMENODE_DISK_ID"), func(ctx context.Context, file *os.File, instance supervisor.Instance) error {
			received = file
			if instance.DataBytes != 16<<20 || instance.State != "stopped" || instance.Workload != "files" {
				t.Fatal("wrong disk inventory")
			}
			data := make([]byte, 6)
			if _, err := file.ReadAt(data, 0); err != nil || string(data) != "pinned" {
				t.Fatal("wrong descriptor data", err)
			}
			if _, err := file.WriteAt([]byte("change"), 0); err == nil {
				t.Fatal("writable descriptor")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if received == nil {
			t.Fatal("copy not invoked")
		}
		if _, err := received.Stat(); err == nil {
			t.Fatal("descriptor retained after acknowledgement")
		}
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_DISK_CLIENT_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-disk-client-")
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
	sourceBinary, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceBinary.Close()
	binary := filepath.Join(directory, "client.test")
	targetBinary, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(targetBinary, sourceBinary)
	closeErr := targetBinary.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	private := filepath.Join(directory, "private")
	if err = os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(private, "disk.raw")
	writer, err := os.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Truncate(16 << 20); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if _, err = writer.WriteAt([]byte("pinned"), 0); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	file, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	socket := filepath.Join(directory, "disk.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: socket})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chown(socket, 0, 1003); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socket, 0660); err != nil {
		t.Fatal(err)
	}
	id, token := state.Random(), state.Random()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.AcceptUnix()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		uid, err := supervisor.PeerUID(connection)
		if err != nil || uid != 1003 {
			done <- disktransport.ErrPacket
			return
		}
		data, err := disktransport.ReceivePacket(ctx, connection)
		if err != nil {
			done <- err
			return
		}
		request, err := disktransport.DecodeRequest(data)
		if err != nil || request.Token != token || request.InstanceID != id {
			done <- disktransport.ErrPacket
			return
		}
		metadata, err := json.Marshal(disktransport.Metadata{Version: 1, InstanceID: id, Workload: "files", Bytes: 16 << 20, ImageSHA256: strings.Repeat("a", 64), DataSchema: 1, Protocol: 1})
		if err != nil {
			done <- err
			return
		}
		done <- disktransport.SendAndWait(ctx, connection, metadata, file)
	}()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestNativeBackupDiskClient$", "-test.v")
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1003, Gid: 1003, Groups: []uint32{}}}
	command.Env = []string{"HOMENODE_DISK_CLIENT_CHILD=1", "HOMENODE_DISK_SOURCE=" + source, "HOMENODE_DISK_SOCKET=" + socket, "HOMENODE_DISK_TOKEN=" + token, "HOMENODE_DISK_ID=" + id}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("backup child: %v: %s", err, output)
	}
	if err = <-done; err != nil {
		t.Fatal("copy acknowledgement", err)
	}
	if _, err = file.Stat(); err != nil {
		t.Fatal("sender descriptor unexpectedly closed", err)
	}
}
