//go:build linux

package disktransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func packetPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "disk.sock")}
	listener, err := net.ListenUnix("unixpacket", address)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	sender, err := net.DialUnix("unixpacket", nil, address)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := listener.AcceptUnix()
	if err != nil {
		sender.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { sender.Close(); receiver.Close() })
	return sender, receiver
}
func TestReadonlyDescriptorPacketPinsOriginalFile(t *testing.T) {
	sender, receiver := packetPair(t)
	path := filepath.Join(t.TempDir(), "volume.raw")
	if err := os.WriteFile(path, []byte("original disk"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = SendFile(ctx, sender, []byte(`{"version":1}`), file); err != nil {
		t.Fatal(err)
	}
	metadata, received, err := ReceiveFile(ctx, receiver)
	if err != nil {
		t.Fatal(err)
	}
	defer received.Close()
	if string(metadata) != `{"version":1}` {
		t.Fatal("metadata changed")
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(received)
	if err != nil || string(data) != "original disk" {
		t.Fatal("descriptor followed replacement", err)
	}
	if _, err = received.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("writable disk descriptor")
	}
	flags, err := unix.FcntlInt(received.Fd(), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("inheritable descriptor", err)
	}
}
func TestRejectedPacketsCloseReceivedDescriptors(t *testing.T) {
	sender, receiver := packetPair(t)
	path := filepath.Join(t.TempDir(), "disk")
	writable, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	readonlyFile, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonlyFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = SendFile(ctx, sender, []byte("metadata"), writable); !errors.Is(err, ErrPacket) {
		t.Fatal("sender admitted writable file", err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		for _, packet := range []struct {
			data   []byte
			rights []byte
		}{
			{[]byte("metadata"), unix.UnixRights(int(readonlyFile.Fd()), int(readonlyFile.Fd()))},
			{bytes.Repeat([]byte("x"), MaxPacket+1), unix.UnixRights(int(readonlyFile.Fd()))},
			{[]byte{0xff}, unix.UnixRights(int(readonlyFile.Fd()))},
			{[]byte("metadata"), unix.UnixRights(int(writable.Fd()))},
			{[]byte("metadata"), nil},
		} {
			if _, _, err = sender.WriteMsgUnix(packet.data, packet.rights, nil); err != nil {
				t.Fatal(err)
			}
			_, file, err := ReceiveFile(ctx, receiver)
			if file != nil {
				file.Close()
				t.Fatal("rejected packet returned file")
			}
			if !errors.Is(err, ErrPacket) {
				t.Fatal("unsafe packet accepted", err)
			}
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) != len(before) {
		t.Fatal("rejected packet leaked descriptor", len(before), len(after), err)
	}
}
func TestDescriptorReceiveCancellation(t *testing.T) {
	_, receiver := packetPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, file, err := ReceiveFile(ctx, receiver); err == nil || file != nil {
		t.Fatal("pending receive accepted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("receive ignored cancellation")
	}
}
