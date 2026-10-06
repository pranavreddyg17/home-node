package transfer

import (
	"errors"
	"net"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestConnectionRefusesNonUnixAndInvalidIdentity(t *testing.T) {
	first, second := net.Pipe()
	defer first.Close()
	defer second.Close()
	for _, uid := range []uint32{0, 1001, 1 << 31} {
		if err := authenticateGuestConnection(first, uid); !errors.Is(err, supervisor.ErrPolicy) {
			t.Fatal("unqualified guest connection accepted", uid, err)
		}
	}
}
