//go:build linux

package transfer

import (
	"bytes"
	"context"
	"encoding/json"
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

type budgetInspector struct{ deadline time.Time }

func (b *budgetInspector) Apply(ctx context.Context, _ supervisor.Request) (supervisor.Instance, error) {
	b.deadline, _ = ctx.Deadline()
	return supervisor.Instance{State: "stopped"}, nil
}

func TestTransferBudgetsReachInspectionAndPreserveCallerDeadline(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "hn-budget-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: filepath.Join(directory, "peer.sock")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	peer, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	for _, operation := range []string{"health", "download", "stat", "finalize"} {
		for _, shortParent := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/normal", true: "/caller-deadline"}[shortParent], func(t *testing.T) {
				inspector := &budgetInspector{}
				service := New(inspector, directory, uint32(os.Geteuid()), 1)
				frame := runtimeclient.GuestRequest{InstanceID: state.Random(), Request: guestproto.Request{Version: 1, RequestID: state.Random(), Operation: operation, ObjectID: state.Random()}}
				if operation == "finalize" {
					frame.Request.Size = 1 << 30
					frame.Request.SHA256 = strings.Repeat("a", 64)
				}
				encoded, err := json.Marshal(frame)
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				var parentDeadline time.Time
				if shortParent {
					parentDeadline = time.Now().Add(10 * time.Second)
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, parentDeadline)
					defer cancel()
				}
				req := httptest.NewRequest("POST", "/v1/guest", bytes.NewReader(encoded))
				req = req.WithContext(supervisor.PeerContext(ctx, peer))
				response := httptest.NewRecorder()
				service.ServeHTTP(response, req)
				if response.Code != 409 || inspector.deadline.IsZero() {
					t.Fatal("authenticated request did not reach bounded runtime inspection", response.Code)
				}
				if shortParent {
					if !inspector.deadline.Equal(parentDeadline) {
						t.Fatal("request extended caller deadline")
					}
					return
				}
				remaining := time.Until(inspector.deadline)
				if operation == "stat" || operation == "finalize" {
					if remaining < 60*time.Second || remaining > ServerWriteTimeout {
						t.Fatal("whole-object verification budget is unsafe", remaining)
					}
				} else if remaining <= 0 || remaining > 20*time.Second {
					t.Fatal("ordinary chunk budget changed", remaining)
				}
			})
		}
	}
}
