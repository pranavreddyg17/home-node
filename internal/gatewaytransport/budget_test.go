package gatewaytransport

import (
	"net/netip"
	"sync"
	"testing"
)

func TestRequestBudgetPreservesOtherDeviceCapacity(t *testing.T) {
	b := requestBudget{perAddress: make(map[netip.Addr]int)}
	first := netip.MustParseAddr("100.64.0.2")
	second := netip.MustParseAddr("100.64.0.3")
	for range 8 {
		if !b.acquire(first) {
			t.Fatal("device budget too small")
		}
	}
	if b.acquire(first) {
		t.Fatal("one device exceeded request budget")
	}
	if !b.acquire(second) {
		t.Fatal("one device exhausted another device's capacity")
	}
	b.release(second)
	for range 8 {
		b.release(first)
	}
	if b.total != 0 || len(b.perAddress) != 0 {
		t.Fatal("idle devices retained")
	}
	if !b.acquire(first) {
		t.Fatal("released capacity unavailable")
	}
	b.release(first)
}

func TestRequestBudgetConcurrentGlobalLimit(t *testing.T) {
	b := requestBudget{perAddress: make(map[netip.Addr]int)}
	var wg sync.WaitGroup
	admitted := make(chan netip.Addr, 128)
	for i := range 128 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			address := netip.AddrFrom4([4]byte{100, 64, 0, byte(i + 1)})
			if b.acquire(address) {
				admitted <- address
			}
		}()
	}
	wg.Wait()
	close(admitted)
	if len(admitted) != 64 || b.total != 64 || len(b.perAddress) != 64 {
		t.Fatal("global request capacity not bounded", len(admitted), b.total)
	}
	for address := range admitted {
		b.release(address)
	}
	if b.total != 0 || len(b.perAddress) != 0 {
		t.Fatal("global capacity leaked")
	}
}
