package install

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// GuestStorageProvisioningPlan joins independently observed device authority
// with the owned installer identity. It is a proposal, not published policy.
type GuestStorageProvisioningPlan struct {
	Identity   GuestUIDProvisioningPlan `json:"identity"`
	GuestGID   uint32                   `json:"guestGid"`
	ParentMode uint32                   `json:"parentMode"`
	ImageMode  uint32                   `json:"imageMode"`
	VolumeMode uint32                   `json:"volumeMode"`
}

// PlanInstalledGuestStorageProvisioning qualifies the proposed sole-group
// storage layout without changing existing configuration, disks or accounts.
func (e *Engine) PlanInstalledGuestStorageProvisioning(ctx context.Context, pool supervisor.GuestUIDPool) (GuestStorageProvisioningPlan, error) {
	empty := GuestStorageProvisioningPlan{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return empty, ErrAccounts
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !e.mu.TryLock() {
		return empty, ErrConflict
	}
	defer e.mu.Unlock()
	return e.planInstalledGuestStorageProvisioningLocked(ctx, pool)
}

func (e *Engine) planInstalledGuestStorageProvisioningLocked(ctx context.Context, pool supervisor.GuestUIDPool) (GuestStorageProvisioningPlan, error) {
	empty := GuestStorageProvisioningPlan{}
	group, err := supervisor.ObserveGuestKVMGroup(ctx)
	if err != nil {
		return empty, fmt.Errorf("qualify KVM storage group: %w", err)
	}
	identity, err := e.planInstalledGuestUIDProvisioningLocked(ctx, pool)
	if err != nil {
		return empty, fmt.Errorf("qualify installed guest UID pool: %w", err)
	}
	accounts, err := e.loadAccountJournal()
	if err != nil {
		return empty, err
	}
	if !accounts.Ready || accounts.OwnerID != identity.OwnerID {
		return empty, ErrConflict
	}
	protected := []int{accounts.Accounts.ControllerGID, accounts.Accounts.TransferGID, accounts.Accounts.RuntimeGID}
	maintenance, err := e.loadMaintenanceAccountJournal(accounts)
	if err != nil && !os.IsNotExist(err) {
		return empty, err
	}
	if err == nil {
		if !maintenance.Ready {
			return empty, ErrConflict
		}
		protected = append(protected, maintenance.Plan.Identity.GID)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, group, protected)
	if err != nil {
		return empty, err
	}
	current, err := supervisor.ObserveGuestKVMGroup(ctx)
	if err != nil {
		return empty, fmt.Errorf("recheck KVM storage group: %w", err)
	}
	if current != group {
		return empty, ErrConflict
	}
	return plan, nil
}

func guestStorageProvisioningPlan(ctx context.Context, identity GuestUIDProvisioningPlan, group uint32, protected []int) (GuestStorageProvisioningPlan, error) {
	empty := GuestStorageProvisioningPlan{}
	canonical, err := guestUIDProvisioningPlan(ctx, identity.OwnerID, supervisor.GuestUIDPool{First: identity.First, Last: identity.Last}, identity.ServiceUIDs)
	if err != nil {
		return empty, err
	}
	if !reflect.DeepEqual(canonical, identity) {
		return empty, ErrConflict
	}
	if group == 0 || group > 1<<31-1 || len(protected) < 3 {
		return empty, ErrAccounts
	}
	for _, gid := range protected {
		if gid <= 0 || gid > 1<<31-1 || uint32(gid) == group {
			return empty, ErrAccounts
		}
	}
	return GuestStorageProvisioningPlan{Identity: canonical, GuestGID: group, ParentMode: 0710, ImageMode: 0440, VolumeMode: 0600}, nil
}
