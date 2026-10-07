package install

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// PlanInstalledGuestUIDProvisioning derives owner and service identities from
// completed installer journals and rechecks their live owned account attributes.
// It is read-only and grants no pool publication or guest activation authority.
func (e *Engine) PlanInstalledGuestUIDProvisioning(ctx context.Context, pool supervisor.GuestUIDPool) (GuestUIDProvisioningPlan, error) {
	empty := GuestUIDProvisioningPlan{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return empty, ErrAccounts
	}
	return e.planInstalledGuestUIDProvisioning(ctx, pool)
}

func (e *Engine) planInstalledGuestUIDProvisioning(ctx context.Context, pool supervisor.GuestUIDPool) (GuestUIDProvisioningPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	empty := GuestUIDProvisioningPlan{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if !e.mu.TryLock() {
		return empty, ErrConflict
	}
	defer e.mu.Unlock()
	base, err := e.loadAccountJournal()
	if err != nil {
		return empty, err
	}
	if !base.Ready {
		return empty, ErrConflict
	}
	backend := nativeAccountProvisioner{}
	snapshot, err := backend.Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return empty, err
	}
	for step := 0; step < 5; step++ {
		matches, err := accountStepMatches(snapshot, base, step)
		if err != nil {
			return empty, err
		}
		if !matches {
			return empty, ErrConflict
		}
	}
	actual, err := backend.Verify(ctx)
	if err != nil {
		return empty, err
	}
	if actual != base.Accounts {
		return empty, ErrConflict
	}
	identities := []uint32{actual.ControllerUID, actual.TransferUID}
	maintenance, err := e.loadMaintenanceAccountJournal(base)
	if err != nil && !os.IsNotExist(err) {
		return empty, err
	}
	if err == nil {
		if !maintenance.Ready {
			return empty, ErrConflict
		}
		for step := 0; step < 2; step++ {
			matches, err := maintenanceAccountStepMatches(snapshot, maintenance.Plan, step)
			if err != nil {
				return empty, err
			}
			if !matches {
				return empty, ErrConflict
			}
		}
		actualBackup, err := InspectMaintenanceAccount(ctx)
		if err != nil {
			return empty, err
		}
		if actualBackup != maintenance.Plan.Identity {
			return empty, ErrConflict
		}
		identities = append(identities, actualBackup.UID)
	}
	return PlanGuestUIDProvisioning(ctx, base.OwnerID, pool, identities)
}
