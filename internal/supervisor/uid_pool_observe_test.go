package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestGuestUIDPoolEligibilityRefusesBeforeHostObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := GuestUIDPool{First: 200000, Last: 200001, Blocked: map[uint32]bool{200000: true}}
	observed, err := ObserveGuestUIDPoolEligibility(ctx, pool)
	if !errors.Is(err, context.Canceled) || observed.First != 0 || observed.Blocked != nil {
		t.Fatal("cancelled observation exposed partial authority", observed, err)
	}
	observed, err = ObserveGuestUIDPoolEligibility(context.Background(), GuestUIDPool{First: 1, Last: 2})
	if !errors.Is(err, ErrPolicy) || observed.First != 0 || observed.Blocked != nil {
		t.Fatal("invalid range reached host observation", observed, err)
	}
	if !pool.Blocked[200000] || len(pool.Blocked) != 1 {
		t.Fatal("input conflicts modified")
	}
}
