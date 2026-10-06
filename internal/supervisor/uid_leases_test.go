package supervisor

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"sync"
	"testing"
)

func TestGuestUIDReservationsPersistAndRefuseConflicts(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	pool := GuestUIDPool{First: 200000, Last: 200002, Blocked: map[uint32]bool{200000: true}}
	id := state.Random()
	uid, err := m.ReserveGuestUID(ctx, id, pool)
	if err != nil || uid != 200001 {
		t.Fatal("host conflict not skipped", uid, err)
	}
	replacement := &Manager{Store: m.Store}
	if got, err := replacement.ReserveGuestUID(ctx, id, pool); err != nil || got != uid {
		t.Fatal("lease replay changed", got, err)
	}
	second := state.Random()
	if got, err := m.ReserveGuestUID(ctx, second, pool); err != nil || got != 200002 {
		t.Fatal("guest identity reused UID", got, err)
	}
	if _, err := m.ReserveGuestUID(ctx, state.Random(), pool); !errors.Is(err, ErrCapacity) {
		t.Fatal("exhausted pool admitted", err)
	}
	pool.Blocked[uid] = true
	if _, err := m.ReserveGuestUID(ctx, id, pool); !errors.Is(err, ErrPolicy) {
		t.Fatal("conflicted existing lease admitted", err)
	}
	changed := GuestUIDPool{First: 200000, Last: 200003}
	if _, err := m.ReserveGuestUID(ctx, state.Random(), changed); !errors.Is(err, ErrPolicy) {
		t.Fatal("changed pool admitted", err)
	}
	var count int
	if err := m.Store.DB.QueryRow("SELECT count(*) FROM runtime_uid_leases").Scan(&count); err != nil || count != 2 {
		t.Fatal("refusal altered lease inventory", count, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.ReserveGuestUID(cancelled, state.Random(), pool); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, invalid := range []GuestUIDPool{{First: 0, Last: 1}, {First: 65536, Last: 65535}, {First: 65536, Last: 131072}} {
		if _, err := m.ReserveGuestUID(ctx, state.Random(), invalid); !errors.Is(err, ErrPolicy) {
			t.Fatal("invalid pool admitted", invalid, err)
		}
	}
}

func TestConcurrentGuestUIDReservationsRemainUnique(t *testing.T) {
	m, _ := newManager(t)
	const count = 16
	pool := GuestUIDPool{First: 200000, Last: 200000 + count - 1}
	type reservation struct {
		id  string
		uid uint32
		err error
	}
	results := make(chan reservation, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			id := state.Random()
			uid, err := m.ReserveGuestUID(context.Background(), id, pool)
			results <- reservation{id, uid, err}
		}()
	}
	workers.Wait()
	close(results)
	seen := make(map[uint32]bool, count)
	for result := range results {
		if result.err != nil || seen[result.uid] || result.uid < pool.First || result.uid > pool.Last {
			t.Fatal("concurrent lease collision/refusal", result)
		}
		seen[result.uid] = true
		if uid, err := m.ReserveGuestUID(context.Background(), result.id, pool); err != nil || uid != result.uid {
			t.Fatal("concurrent lease retry changed", uid, err)
		}
	}
	if len(seen) != count {
		t.Fatal("incomplete concurrent inventory", seen)
	}
	if _, err := m.ReserveGuestUID(context.Background(), state.Random(), pool); !errors.Is(err, ErrCapacity) {
		t.Fatal("concurrent allocation exceeded pool", err)
	}
}
