package install

import "context"

// Inspect the owned installation independently of reserved-pool NSS eligibility:
// the identity preparation transaction is what establishes that stricter policy.
// This grants no file publication or allocation authority. Caller holds e.mu.
func (e *Engine) inspectGuestIdentityAccountsLocked(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	base, err := e.loadAccountJournal()
	if err != nil {
		return "", err
	}
	if !base.Ready {
		return "", ErrConflict
	}
	maintenance, err := e.loadMaintenanceAccountJournal(base)
	if err != nil {
		return "", err
	}
	if !maintenance.Ready {
		return "", ErrConflict
	}
	backend := nativeAccountProvisioner{}
	snapshot, err := backend.Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return "", err
	}
	for step := 0; step < 5; step++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		matches, err := e.ownedAccountStepMatches(snapshot, base, step)
		if err != nil {
			return "", err
		}
		if !matches {
			return "", ErrConflict
		}
	}
	for step := 0; step < 2; step++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		matches, err := maintenanceAccountStepMatches(snapshot, maintenance.Plan, step)
		if err != nil {
			return "", err
		}
		if !matches {
			return "", ErrConflict
		}
	}
	actual, err := backend.Verify(ctx)
	if err != nil {
		return "", err
	}
	if actual != base.Accounts {
		return "", ErrConflict
	}
	actualMaintenance, err := InspectMaintenanceAccount(ctx)
	if err != nil {
		return "", err
	}
	if actualMaintenance != maintenance.Plan.Identity {
		return "", ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return base.OwnerID, nil
}
