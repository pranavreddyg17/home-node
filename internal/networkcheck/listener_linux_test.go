//go:build linux

package networkcheck

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDeviceBoundListener(t *testing.T) {
	device, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := listenOnDevice(context.Background(), "127.0.0.1:0", device)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tcp := listener.(*net.TCPListener)
	raw, err := tcp.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var index int
	var readErr error
	if err := raw.Control(func(fd uintptr) {
		index, readErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BINDTOIFINDEX)
	}); err != nil || readErr != nil || index != device.Index {
		t.Fatalf("listener lost device binding: index=%d control=%v read=%v", index, err, readErr)
	}
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := tcp.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	accepted, err := tcp.Accept()
	if err != nil {
		t.Fatal(err)
	}
	accepted.Close()
}

func TestDeviceBindingFailureNeverReturnsListener(t *testing.T) {
	device, err := net.InterfaceByName("lo")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []*net.Interface{
		nil,
		{Index: device.Index, Name: "hn-no-device"},
		{Index: device.Index + 100000, Name: device.Name},
	} {
		listener, err := listenOnDevice(context.Background(), "127.0.0.1:0", bad)
		if err == nil || listener != nil {
			if listener != nil {
				listener.Close()
			}
			t.Fatal("failed interface enforcement returned an open listener")
		}
	}
}

func TestPrivateListenerRejectsLoopbackConfiguration(t *testing.T) {
	listener, err := ListenTailnet(context.Background(), Config{Bind: "127.0.0.1", Port: 8787, Origin: "https://home.example.ts.net:8787"})
	if err == nil || listener != nil {
		t.Fatal("production listener accepted loopback configuration")
	}
}
