package supervisor

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestGuestUIDReservationsExcludeLocalAndDelegatedAccounts(t *testing.T) {
	original := GuestUIDPool{First: 200000, Last: 200004, Blocked: map[uint32]bool{200004: true}}
	passwd := []byte("root:x:0:0:root:/root:/bin/sh\nother:x:200000:1000:owner:/home/owner:/bin/sh\n")
	pool, err := accountGuestUIDConflicts(context.Background(), original, passwd, []byte("owner:200001:2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if original.Blocked[200000] || original.Blocked[200001] {
		t.Fatal("caller policy mutated")
	}
	m, _ := newManager(t)
	if uid, err := m.ReserveGuestUID(context.Background(), state.Random(), pool); err != nil || uid != 200003 {
		t.Fatal("host/delegated UID collision", uid, err)
	}
	if _, err := m.ReserveGuestUID(context.Background(), state.Random(), pool); !errors.Is(err, ErrCapacity) {
		t.Fatal("blocked pool reused", err)
	}
	for _, subuid := range []string{"owner:4294967295:2\n", "owner:200000:0\n", "owner:-1:2\n", "owner:200000:2:extra\n", "owner:200000:2\r\n"} {
		if _, err := accountGuestUIDConflicts(context.Background(), original, passwd, []byte(subuid)); !errors.Is(err, ErrPolicy) {
			t.Fatal("ambiguous delegation accepted", subuid, err)
		}
	}
	for _, accounts := range []string{"", "other:x:-1:0:o:/h:/s\n", "other:x:4294967296:0:o:/h:/s\n", "other:x:1:o:/h:/s\n"} {
		if _, err := accountGuestUIDConflicts(context.Background(), original, []byte(accounts), nil); !errors.Is(err, ErrPolicy) {
			t.Fatal("ambiguous account accepted", accounts, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := accountGuestUIDConflicts(ctx, original, passwd, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
