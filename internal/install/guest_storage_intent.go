package install

import (
	"context"
	"encoding/json"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"os"
	"reflect"
	"runtime"
	"time"
)

type guestStorageIntent struct {
	Version int                          `json:"version"`
	Plan    GuestStorageProvisioningPlan `json:"plan"`
}

// PrepareGuestStorageProvisioning records qualified immutable proposal only.
// It does not change installed storage permissions or publish runtime policy.
func (e *Engine) PrepareGuestStorageProvisioning(ctx context.Context, pool supervisor.GuestUIDPool) (GuestStorageProvisioningPlan, error) {
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
	plan, err := e.planInstalledGuestStorageProvisioningLocked(ctx, pool)
	if err != nil {
		return empty, err
	}
	if err := e.commitGuestStorageIntent(ctx, plan); err != nil {
		return empty, err
	}
	return plan, nil
}

func (e *Engine) commitGuestStorageIntent(ctx context.Context, plan GuestStorageProvisioningPlan) error {
	qualified, err := canonicalGuestStoragePlan(ctx, plan)
	if err != nil {
		return err
	}
	data, err := json.Marshal(guestStorageIntent{Version: 1, Plan: qualified})
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-storage-intent.json", "guest-storage-intent", data)
}

func canonicalGuestStoragePlan(ctx context.Context, plan GuestStorageProvisioningPlan) (GuestStorageProvisioningPlan, error) {
	identity, err := guestUIDProvisioningPlan(ctx, plan.Identity.OwnerID, supervisor.GuestUIDPool{First: plan.Identity.First, Last: plan.Identity.Last}, plan.Identity.ServiceUIDs)
	if err != nil {
		return GuestStorageProvisioningPlan{}, err
	}
	if !reflect.DeepEqual(identity, plan.Identity) || plan.GuestGID == 0 || plan.GuestGID > 1<<31-1 || plan.ParentMode != 0710 || plan.ImageMode != 0440 || plan.VolumeMode != 0600 {
		return GuestStorageProvisioningPlan{}, ErrPlan
	}
	plan.Identity = identity
	return plan, nil
}
