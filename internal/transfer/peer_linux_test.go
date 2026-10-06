//go:build linux

package transfer

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestConnectionAuthenticatesKernelPeerUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	uid := uint32(os.Geteuid())
	err = authenticateGuestConnection(connection, uid)
	if uid == 0 {
		if !errors.Is(err, supervisor.ErrPolicy) {
			t.Fatal("root guest peer accepted", err)
		}
	} else if err != nil {
		t.Fatal("matching kernel peer refused", err)
	}
	if err := authenticateGuestConnection(connection, uid+1); !errors.Is(err, supervisor.ErrPolicy) {
		t.Fatal("wrong guest UID accepted", err)
	}
	if err := authenticateGuestConnection(connection, 0); !errors.Is(err, supervisor.ErrPolicy) {
		t.Fatal("missing guest UID accepted", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := authenticateGuestConnection(connection, 1001); !errors.Is(err, supervisor.ErrPolicy) {
		t.Fatal("closed guest connection accepted", err)
	}
}
