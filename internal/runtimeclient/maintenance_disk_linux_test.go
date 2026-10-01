//go:build linux

package runtimeclient

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
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestDiskClientRefusesNonRootPeerBeforeCopy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires a non-root fixture listener")
	}
	path := filepath.Join(t.TempDir(), "disk.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.AcceptUnix()
		if err == nil {
			connection.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	invoked := false
	err = NewDiskClient(path).WithMaintenanceDisk(ctx, state.Random(), state.Random(), func(context.Context, *os.File, supervisor.Instance) error { invoked = true; return nil })
	if !errors.Is(err, disktransport.ErrPacket) || invoked {
		t.Fatal("foreign root service accepted", invoked, err)
	}
	<-done
}
