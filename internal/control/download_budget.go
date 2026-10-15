package control

import "sync"

// Full-object integrity verification is required even for short ranges. Bound
// that work independently of generic HTTP slots so one paired device cannot
// occupy every download or management slot. Entries exist only while active.
type downloadBudget struct {
	mu        sync.Mutex
	total     int
	perDevice map[string]int
}

func (b *downloadBudget) acquire(device string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if device == "" || b.total >= 4 || b.perDevice[device] >= 2 {
		return false
	}
	if b.perDevice == nil {
		b.perDevice = make(map[string]int)
	}
	b.total++
	b.perDevice[device]++
	return true
}

func (b *downloadBudget) release(device string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total--
	b.perDevice[device]--
	if b.perDevice[device] == 0 {
		delete(b.perDevice, device)
	}
}
