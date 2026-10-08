//go:build !linux

package install

import "testing"

func applyAccountFixtureAllocator(t *testing.T, _ **Engine, _ GuestUIDAllocationPreview) {
	t.Helper()
	t.Fatal("native allocator fixture requires Linux")
}
