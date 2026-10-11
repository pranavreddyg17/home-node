package gatewaytransport

import (
	"net/netip"
	"sync"
)

// HTTP/2 can multiplex requests inside one accepted connection. TCP limits
// alone therefore cannot prevent one device occupying every controller slot.
// Entries exist only for admitted requests, keeping this map bounded by 64.
type requestBudget struct {
	mu         sync.Mutex
	total      int
	perAddress map[netip.Addr]int
}

func (b *requestBudget) acquire(address netip.Addr) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total >= 64 || b.perAddress[address] >= 8 {
		return false
	}
	b.total++
	b.perAddress[address]++
	return true
}

func (b *requestBudget) release(address netip.Addr) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total--
	b.perAddress[address]--
	if b.perAddress[address] == 0 {
		delete(b.perAddress, address)
	}
}
