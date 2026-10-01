//go:build linux

package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func dispatchPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "dispatch.sock")}
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
func TestDispatchReceiverKernelIdentityAndPacketAdmission(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("controller must be unprivileged")
	}
	dispatch := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err := EncodeDispatch(dispatch)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "foreign-peer", "root-peer", "descriptor", "malformed", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			expected := uid
			packet := raw
			var ancillary []byte
			switch scenario {
			case "foreign-peer":
				expected = uid + 1
			case "root-peer":
				expected = 0
			case "descriptor":
				file, err := os.Open("/dev/null")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				ancillary = unix.UnixRights(int(file.Fd()))
			case "malformed":
				packet = []byte(`{"version":1}`)
			case "oversized":
				packet = make([]byte, MaxDispatchBytes+1)
			}
			if _, _, err := sender.WriteMsgUnix(packet, ancillary, nil); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := ReceiveDispatch(ctx, receiver, expected)
			if scenario == "valid" {
				if err != nil || result != dispatch {
					t.Fatal("valid dispatch refused", err)
				}
			} else if err == nil || result != (Dispatch{}) {
				t.Fatal("invalid dispatch admitted", scenario)
			}
		})
	}
	sender, receiver := dispatchPair(t)
	_ = sender
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := ReceiveDispatch(ctx, receiver, uid); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled receive remained blocked")
	}
}

func TestDispatchSenderAuthenticatesBeforeSendingJob(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("backup peer must be unprivileged")
	}
	dispatch := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	for _, scenario := range []string{"valid", "foreign-peer", "root-peer", "invalid-job", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			expected := uid
			message := dispatch
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			switch scenario {
			case "foreign-peer":
				expected = uid + 1
			case "root-peer":
				expected = 0
			case "invalid-job":
				message.JobID = "short"
			case "cancelled":
				cancel()
			}
			err := SendDispatch(ctx, sender, expected, message)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				received, err := ReceiveDispatch(ctx, receiver, uid)
				if err != nil || received != dispatch {
					t.Fatal("authenticated dispatch lost", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid sender admitted")
				}
				if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation lost", err)
				}
				if err = receiver.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				data := make([]byte, MaxDispatchBytes)
				if n, _, _, _, err := receiver.ReadMsgUnix(data, nil); n != 0 || err == nil {
					t.Fatal("refused sender exposed job packet")
				}
			}
		})
	}
}
