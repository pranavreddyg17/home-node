//go:build linux

package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type peerFixtureInspector struct{ instance supervisor.Instance }

func (f peerFixtureInspector) Apply(context.Context, supervisor.Request) (supervisor.Instance, error) {
	return f.instance, nil
}

func TestTransferRefusedGuestReceivesNoRequestBytes(t *testing.T) {
	for _, scenario := range []string{"reserved-new", "reserved-cached", "shared-new", "stopped-cached"} {
		t.Run(scenario, func(t *testing.T) {
			root, err := os.MkdirTemp("/tmp", "hn-peer-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(root); err != nil {
					t.Errorf("fixture cleanup: %v", err)
				}
			})
			controller, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(root, "controller.sock")})
			if err != nil {
				t.Fatal(err)
			}
			defer controller.Close()
			client, err := net.DialUnix("unix", nil, controller.Addr().(*net.UnixAddr))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			controllerPeer, err := controller.AcceptUnix()
			if err != nil {
				t.Fatal(err)
			}
			defer controllerPeer.Close()
			id := state.Random()
			directory := filepath.Join(root, id)
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(directory, "adapter.sock")})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			actualUID := uint32(os.Geteuid())
			instance := supervisor.Instance{ID: id, State: "running", GuestUID: actualUID + 1}
			if scenario == "stopped-cached" {
				instance.State = "stopped"
			}
			if scenario == "shared-new" {
				instance.GuestUID = 0
			}
			service := New(peerFixtureInspector{instance}, root, actualUID, 1)
			service.SharedGuestUID = actualUID + 1
			hash := fnv.New32a()
			_, _ = hash.Write([]byte(id))
			index := hash.Sum32() % 16
			if strings.HasSuffix(scenario, "-cached") {
				cached, err := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
				if err != nil {
					t.Fatal(err)
				}
				defer cached.Close()
				service.connections[index] = cached
				service.instanceIDs[index] = id
			}
			frame := runtimeclient.GuestRequest{InstanceID: id, Request: guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"}}
			body, err := json.Marshal(frame)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/v1/guest", bytes.NewReader(body))
			request = request.WithContext(supervisor.PeerContext(request.Context(), controllerPeer))
			response := httptest.NewRecorder()
			service.ServeHTTP(response, request)
			wantStatus := 503
			if scenario == "stopped-cached" {
				wantStatus = 409
			}
			if response.Code != wantStatus {
				t.Fatal("guest identity refusal missing", response.Code, response.Body.String())
			}
			if service.connections[index] != nil {
				t.Fatal("refused connection retained")
			}
			if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			guestPeer, err := listener.AcceptUnix()
			if err != nil {
				t.Fatal(err)
			}
			defer guestPeer.Close()
			if err := guestPeer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var data [1]byte
			n, err := guestPeer.Read(data[:])
			if n != 0 || err != io.EOF {
				t.Fatal("request bytes reached wrong guest peer", n, err)
			}
		})
	}
}

func TestTransferAdmittedPeerRoundTripAndCachedIdentityDrift(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("positive guest peer must be non-root")
	}
	root, err := os.MkdirTemp("/tmp", "hn-peer-ok-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	controller, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(root, "controller.sock")})
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	client, err := net.DialUnix("unix", nil, controller.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	controllerPeer, err := controller.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer controllerPeer.Close()
	id := state.Random()
	directory := filepath.Join(root, id)
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(directory, "adapter.sock")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	service := New(peerFixtureInspector{supervisor.Instance{ID: id, State: "running", GuestUID: uid}}, root, uid, 1)
	defer func() {
		for _, connection := range service.connections {
			if connection != nil {
				connection.Close()
			}
		}
	}()
	done := make(chan error, 1)
	go func() {
		peer, err := listener.AcceptUnix()
		if err != nil {
			done <- err
			return
		}
		defer peer.Close()
		if err := peer.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			done <- err
			return
		}
		for i := 0; i < 2; i++ {
			var request guestproto.Request
			if err := guestproto.Read(peer, &request); err != nil {
				done <- err
				return
			}
			if request.Operation != "health" {
				done <- fmt.Errorf("unexpected operation")
				return
			}
			if err := guestproto.Write(peer, guestproto.Response{Version: 1, RequestID: request.RequestID, State: "ready"}); err != nil {
				done <- err
				return
			}
		}
		var data [1]byte
		n, err := peer.Read(data[:])
		if n != 0 || err != io.EOF {
			done <- fmt.Errorf("identity drift delivered bytes: %d %v", n, err)
			return
		}
		done <- nil
	}()
	for i := 0; i < 3; i++ {
		if i == 2 {
			service.Runtime = peerFixtureInspector{supervisor.Instance{ID: id, State: "running", GuestUID: uid + 1}}
		}
		frame := runtimeclient.GuestRequest{InstanceID: id, Request: guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"}}
		body, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "/v1/guest", bytes.NewReader(body))
		request = request.WithContext(supervisor.PeerContext(request.Context(), controllerPeer))
		result := httptest.NewRecorder()
		service.ServeHTTP(result, request)
		if i < 2 {
			if result.Code != 200 {
				t.Fatal("admitted guest request failed", result.Code, result.Body.String())
			}
			var response guestproto.Response
			if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil || response.RequestID != frame.Request.RequestID || response.State != "ready" {
				t.Fatal("round trip response", response, err)
			}
		} else if result.Code != 503 {
			t.Fatal("cached identity drift accepted", result.Code)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("guest fixture did not terminate")
	}
}

func TestTransferCancellationInterruptsGuestRead(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("positive guest peer must be non-root")
	}
	root, err := os.MkdirTemp("/tmp", "hn-peer-cancel-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	controller, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(root, "controller.sock")})
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	client, err := net.DialUnix("unix", nil, controller.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	controllerPeer, err := controller.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer controllerPeer.Close()
	id := state.Random()
	directory := filepath.Join(root, id)
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(directory, "adapter.sock")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		peer, err := listener.AcceptUnix()
		if err != nil {
			done <- err
			return
		}
		defer peer.Close()
		if err := peer.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			done <- err
			return
		}
		var frame guestproto.Request
		if err := guestproto.Read(peer, &frame); err != nil {
			done <- err
			return
		}
		cancel() // The guest deliberately withholds a response.
		var data [1]byte
		n, err := peer.Read(data[:])
		if n != 0 || err != io.EOF {
			done <- fmt.Errorf("cancelled guest connection retained: %d %v", n, err)
			return
		}
		done <- nil
	}()
	service := New(peerFixtureInspector{supervisor.Instance{ID: id, State: "running", GuestUID: uid}}, root, uid, 1)
	frame := runtimeclient.GuestRequest{InstanceID: id, Request: guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "health"}}
	body, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/v1/guest", bytes.NewReader(body)).WithContext(supervisor.PeerContext(ctx, controllerPeer))
	response := httptest.NewRecorder()
	started := time.Now()
	service.ServeHTTP(response, request)
	if response.Code < 400 || time.Since(started) >= 3*time.Second {
		t.Fatal("cancellation waited for guest response deadline", response.Code, time.Since(started))
	}
	for _, connection := range service.connections {
		if connection != nil {
			connection.Close()
			t.Fatal("cancelled connection cached")
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancelled guest fixture did not terminate")
	}
}
