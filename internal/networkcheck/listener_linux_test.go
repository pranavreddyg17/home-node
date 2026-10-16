//go:build linux

package networkcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Invoked only inside the disposable namespace fixture, after dropping all
// capabilities and switching UID. The veth stands in for TUN ingress; this
// exercises the production socket binding, not Tailscale identity admission.
func TestIngressNamespaceListenerChild(t *testing.T) {
	mode := os.Getenv("HOMENODE_INGRESS_FIXTURE_CHILD")
	if mode == "" {
		t.Skip("disposable Linux namespace fixture only")
	}
	if os.Geteuid() != 65534 || (mode != "bound" && mode != "control" && mode != "watched" && mode != "production" && mode != "production-refusal") {
		t.Fatal("fixture child requires its unprivileged identity and known mode")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range []string{"CapInh:", "CapPrm:", "CapEff:", "CapBnd:", "CapAmb:"} {
		found := false
		for line := range strings.SplitSeq(string(status), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == capability {
				found = fields[1] == "0000000000000000"
			}
		}
		if !found {
			t.Fatalf("fixture retained or could not verify %s", capability)
		}
	}
	var listener net.Listener
	if mode == "production" || mode == "production-refusal" {
		configuration := Config{Bind: "100.100.1.2", Port: 8787, Origin: "https://home.example.ts.net:8787"}
		preflightErr := CheckIngressInterface(configuration)
		if (mode == "production" && preflightErr != nil) || (mode == "production-refusal" && !errors.Is(preflightErr, ErrIdentity)) {
			t.Fatalf("read-only interface preflight disagreed with TUN address state: %v", preflightErr)
		}
		listener, err = ListenTailnet(context.Background(), configuration)
		if mode == "production-refusal" {
			if listener != nil {
				listener.Close()
			}
			if !errors.Is(err, ErrIdentity) {
				t.Fatalf("missing TUN address did not refuse production admission: %v", err)
			}
			fmt.Println("ADMISSION-REFUSED")
			return
		}
	} else if mode == "bound" || mode == "watched" {
		device, lookupErr := net.InterfaceByName("tailscale0")
		if lookupErr != nil {
			t.Fatal(lookupErr)
		}
		listener, err = listenOnDevice(context.Background(), "100.100.1.2:0", device)
		if err == nil && mode == "watched" {
			listener = watchListener(listener, 10*time.Millisecond, func() bool {
				current, err := net.InterfaceByName("tailscale0")
				return err == nil && current.Index == device.Index
			})
		}
	} else {
		listener, err = net.Listen("tcp4", "100.100.1.2:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	fmt.Printf("READY %s\n", listener.Addr())
	for {
		connection, err := listener.Accept()
		if err != nil {
			if (mode == "watched" || mode == "production") && errors.Is(err, net.ErrClosed) {
				fmt.Println("WATCH-CLOSED")
				return
			}
			t.Fatal(err)
		}
		connection.SetWriteDeadline(time.Now().Add(time.Second))
		_, err = connection.Write([]byte("homenode-ingress\n"))
		connection.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

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
