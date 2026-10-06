package backup

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"
)

// PreparedRecoveryLease pins private maintenance-owned output for installer
// inspection. expectedUID must come from trusted installed account configuration,
// never restored metadata. This lease grants no activation/ownership authority.
type PreparedRecoveryLease struct {
	ownerUID uint32
	root     *os.Root
	lock     *os.File
	mu       sync.Mutex
	closed   bool
	closeErr error
}

func OpenPreparedRecovery(ctx context.Context, root *os.Root, expectedUID uint32) (*PreparedRecoveryLease, error) {
	if root == nil {
		return nil, ErrManifest
	}
	lock, err := lockPrivateOwnedRoot(ctx, root, expectedUID, false)
	if err != nil {
		return nil, err
	}
	pinned, err := root.OpenRoot(".")
	if err != nil {
		return nil, errors.Join(err, lock.Close())
	}
	before, beforeErr := root.Stat(".")
	after, afterErr := pinned.Stat(".")
	if beforeErr != nil || afterErr != nil || !os.SameFile(before, after) {
		return nil, errors.Join(ErrManifest, pinned.Close(), lock.Close())
	}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, pinned.Close(), lock.Close())
	}
	return &PreparedRecoveryLease{ownerUID: expectedUID, root: pinned, lock: lock}, nil
}

// InventoryForOwner binds the retained lease to the installer's independently
// observed maintenance account. A lease opened for another identity is refused.
func (l *PreparedRecoveryLease) InventoryForOwner(ctx context.Context, expectedUID uint32, manifest Manifest, policy RestorePolicy) (PreparedRecoveryInventory, error) {
	if l == nil || l.ownerUID != expectedUID {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	return l.Inventory(ctx, manifest, policy)
}

func (l *PreparedRecoveryLease) Inventory(ctx context.Context, manifest Manifest, policy RestorePolicy) (PreparedRecoveryInventory, error) {
	if err := ctx.Err(); err != nil {
		return PreparedRecoveryInventory{}, err
	}
	if l == nil {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	if !l.mu.TryLock() {
		return PreparedRecoveryInventory{}, ErrMaintenanceRunner
	}
	defer l.mu.Unlock()
	if l.closed {
		return PreparedRecoveryInventory{}, ErrManifest
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	return InspectPreparedRecovery(deadline, l.root, manifest, policy)
}

func (l *PreparedRecoveryLease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		l.closeErr = errors.Join(l.root.Close(), l.lock.Close())
	}
	return l.closeErr
}
