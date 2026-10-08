//go:build !linux

package install

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func (e *Engine) PrepareGuestUIDAllocationConfiguration(ctx context.Context, _ supervisor.GuestUIDPool, _ GuestUIDAllocatorRanges) (GuestUIDAllocationPreview, error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDAllocationPreview{}, err
	}
	return GuestUIDAllocationPreview{}, ErrAccounts
}
