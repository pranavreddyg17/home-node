package supervisor

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"strings"
	"testing"
)

func TestGuestUIDReservationsExcludeEveryProcessCredential(t *testing.T) {
	original := GuestUIDPool{First: 200000, Last: 200004}
	valid := "Name:\tfixture\nUid:\t200000\t200001\t200002\t200003\nGid:\t0\t0\t0\t0\n"
	pool, err := processGuestUIDConflicts(context.Background(), original, []byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if original.Blocked != nil {
		t.Fatal("caller policy mutated")
	}
	m, _ := newManager(t)
	if uid, err := m.ReserveGuestUID(context.Background(), state.Random(), pool); err != nil || uid != 200004 {
		t.Fatal("process credential reused", uid, err)
	}
	for _, status := range []string{"Name:\tfixture\n", valid + "Uid:\t0\t0\t0\t0\n", strings.Replace(valid, "200003", "-1", 1), strings.Replace(valid, "200003", "4294967296", 1), strings.Replace(valid, "\t200003", "", 1), strings.ReplaceAll(valid, "\n", "\r\n"), strings.Repeat(valid, 2000)} {
		if _, err := processGuestUIDConflicts(context.Background(), original, []byte(status)); !errors.Is(err, ErrPolicy) {
			t.Fatal("ambiguous credentials admitted", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := processGuestUIDConflicts(ctx, original, []byte(valid)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
