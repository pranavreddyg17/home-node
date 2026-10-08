//go:build linux

package install

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"golang.org/x/sys/unix"
)

// PrepareGuestUIDAllocationConfiguration binds explicitly chosen allocator
// ranges to the owned accounts and protected original host configuration. It
// records intent only; source bytes, runtime policy and activation are unchanged.
func (e *Engine) PrepareGuestUIDAllocationConfiguration(ctx context.Context, pool supervisor.GuestUIDPool, selection GuestUIDAllocatorRanges) (preview GuestUIDAllocationPreview, result error) {
	if err := ctx.Err(); err != nil {
		return preview, err
	}
	if os.Geteuid() != 0 || e.host.Name() != "/" {
		return preview, ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !e.mu.TryLock() {
		return preview, ErrConflict
	}
	defer e.mu.Unlock()
	owner, err := e.inspectGuestIdentityAccountsLocked(ctx)
	if err != nil {
		return preview, err
	}
	// Reject unobserved blocked-map overrides; this command selects range bounds
	// and never claims to have established vacancy or allocation exclusivity.
	if len(pool.Blocked) != 0 {
		return preview, ErrPlan
	}
	file, err := e.host.OpenFile("etc/login.defs", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return preview, err
	}
	var metadata unix.Stat_t
	if unix.Fstat(int(file.Fd()), &metadata) != nil || metadata.Mode != unix.S_IFREG|0644 || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Nlink != 1 || metadata.Size <= 0 || metadata.Size > maxGuestUIDAllocatorConfigurationBytes {
		return preview, errors.Join(ErrConflict, file.Close())
	}
	original, readErr := io.ReadAll(io.LimitReader(file, maxGuestUIDAllocatorConfigurationBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return preview, err
	}
	proposal, err := planGuestUIDAllocatorConfiguration(ctx, original, pool, selection)
	if err != nil {
		return preview, err
	}
	intent := guestUIDAllocationIntent{Version: 1, OwnerID: owner, First: pool.First, Last: pool.Last, Selection: selection, Original: string(original), Proposal: proposal}
	result = e.withGuestIdentityConfigurationSource(ctx, "login.defs", intent.Original, func(ctx context.Context, _ *os.File, checkSource func() error) error {
		if err := checkSource(); err != nil {
			return err
		}
		if err := e.commitGuestUIDAllocationIntent(ctx, intent); err != nil {
			return err
		}
		return e.withGuestUIDAllocationIntent(ctx, intent, func(ctx context.Context, checkIntent func() error) error {
			currentOwner, err := e.inspectGuestIdentityAccountsLocked(ctx)
			if err != nil {
				return err
			}
			if currentOwner != owner {
				return ErrConflict
			}
			if err := checkSource(); err != nil {
				return err
			}
			return checkIntent()
		})
	})
	if result != nil {
		return preview, result
	}
	return GuestUIDAllocationPreview{OwnerID: owner, First: pool.First, Last: pool.Last, Selection: selection, OriginalSHA256: proposal.OriginalSHA256, DesiredSHA256: proposal.DesiredSHA256, ChangesRequired: proposal.OriginalSHA256 != proposal.DesiredSHA256, IntentCommitted: true}, nil
}
