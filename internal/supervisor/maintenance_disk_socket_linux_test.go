//go:build linux

package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceDiskSocketRefusesForeignPeerAndAuthority(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("authorized backup peer must be unprivileged")
	}
	for _, scenario := range []string{"foreign-peer", "invalid-request", "foreign-token"} {
		t.Run(scenario, func(t *testing.T) {
			manager, backend := newManager(t)
			manager.Policy.ControllerUID = uid + 1
			manager.Policy.TransferUID = uid + 2
			address := &net.UnixAddr{Net: "unixpacket", Name: filepath.Join(t.TempDir(), "disk.sock")}
			listener, err := net.ListenUnix("unixpacket", address)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			client, err := net.DialUnix("unixpacket", nil, address)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			server, err := listener.AcceptUnix()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			peer := uid
			if scenario == "foreign-peer" {
				peer = uid + 3
			}
			done := make(chan error, 1)
			go func() { done <- manager.ServeMaintenanceDisk(ctx, server, peer) }()
			if scenario != "foreign-peer" {
				request := []byte(`{"version":1,"token":"` + state.Random() + `","instanceId":"` + state.Random() + `"}`)
				if scenario == "invalid-request" {
					request = []byte(`{"version":1,"token":"short","instanceId":"short"}`)
				}
				if err = disktransport.SendPacket(ctx, client, request); err != nil {
					t.Fatal(err)
				}
			}
			err = <-done
			if err == nil {
				t.Fatal("unauthorized disk session accepted")
			}
			if scenario == "foreign-peer" && !errors.Is(err, ErrPolicy) {
				t.Fatal(err)
			}
			if backend.starts != 0 || backend.stops != 0 {
				t.Fatal("refused request changed runtime")
			}
			var barriers int
			if err = manager.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key='runtime.maintenance'").Scan(&barriers); err != nil || barriers != 0 {
				t.Fatal("refused request acquired authority", barriers, err)
			}
		})
	}
}
