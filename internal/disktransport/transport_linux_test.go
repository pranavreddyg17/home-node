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

func TestControlPacketRefusesDescriptorAuthority(t *testing.T) {
	sender, receiver := packetPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message := []byte(`{"version":1,"copied":true}`)
	if err := SendPacket(ctx, sender, message); err != nil {
		t.Fatal(err)
	}
	data, err := ReceivePacket(ctx, receiver)
	if err != nil || !bytes.Equal(data, message) {
		t.Fatal("control packet changed", err)
	}
	path := filepath.Join(t.TempDir(), "disk")
	if err = os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if err = SendFile(ctx, sender, message, file); err != nil {
		t.Fatal(err)
	}
	if _, err = ReceivePacket(ctx, receiver); !errors.Is(err, ErrPacket) {
		t.Fatal("control packet accepted descriptor", err)
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) != len(before) {
		t.Fatal("control rejection leaked descriptor", err)
	}
}

func TestCopySessionAcknowledgesOnlyAfterDescriptorClose(t *testing.T) {
	for _, failCopy := range []bool{false, true} {
		sender, receiver := packetPair(t)
		path := filepath.Join(t.TempDir(), "disk")
		if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 1)
		go func() { done <- SendAndWait(ctx, sender, []byte(`{"version":1}`), file) }()
		failure := errors.New("copy fixture failure")
		var observed *os.File
		err = CopySession(ctx, receiver, func(ctx context.Context, metadata []byte, disk *os.File) error {
			observed = disk
			if failCopy {
				return failure
			}
			data, err := io.ReadAll(disk)
			if err != nil || string(data) != "payload" {
				return ErrPacket
			}
			return nil
		})
		sourceErr := <-done
		cancel()
		if failCopy {
			if !errors.Is(err, failure) || sourceErr == nil {
				t.Fatal("failed copy acknowledged", err, sourceErr)
			}
		} else if err != nil || sourceErr != nil {
			t.Fatal(err, sourceErr)
		}
		if observed == nil {
			t.Fatal("copy not invoked")
		}
		if _, err = observed.Stat(); err == nil {
			t.Fatal("received descriptor retained after acknowledgement")
		}
		if _, err = file.Stat(); err != nil {
			t.Fatal("sender descriptor closed by session", err)
		}
	}
}

func TestCopySessionRefusesFalseAcknowledgement(t *testing.T) {
	sender, receiver := packetPair(t)
	path := filepath.Join(t.TempDir(), "disk")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- SendAndWait(ctx, sender, []byte(`{"version":1}`), file) }()
	_, received, err := ReceiveFile(ctx, receiver)
	if err != nil {
		t.Fatal(err)
	}
	received.Close()
	if err = SendPacket(ctx, receiver, []byte(`{"version":1,"copied":false}`)); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrPacket) {
		t.Fatal("false completion accepted", err)
	}
}
