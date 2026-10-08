package install

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func TestGuestUIDAllocatorPreparationRefusesFixtureHostAndCancellation(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	pool := supervisor.GuestUIDPool{First: 2000000000, Last: 2000000001}
	preview, err := e.PrepareGuestUIDAllocationConfiguration(context.Background(), pool, GuestUIDAllocatorRanges{})
	if !errors.Is(err, ErrAccounts) || preview != (GuestUIDAllocationPreview{}) {
		t.Fatal("fixture host admitted for native allocator preparation", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preview, err = e.PrepareGuestUIDAllocationConfiguration(ctx, pool, GuestUIDAllocatorRanges{})
	if !errors.Is(err, context.Canceled) || preview != (GuestUIDAllocationPreview{}) {
		t.Fatal("canceled allocator preparation supplied authority", err)
	}
}

func TestGuestUIDAllocatorApplicationRefusesFixtureHostAndCancellation(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	preview, err := e.ApplyGuestUIDAllocationConfiguration(context.Background())
	if !errors.Is(err, ErrAccounts) || preview != (GuestUIDAllocationPreview{}) {
		t.Fatal("fixture host admitted for native allocator application", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preview, err = e.ApplyGuestUIDAllocationConfiguration(ctx)
	if !errors.Is(err, context.Canceled) || preview != (GuestUIDAllocationPreview{}) {
		t.Fatal("canceled allocator application supplied authority", err)
	}
}
