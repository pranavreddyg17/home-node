package main

import (
	"context"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Consume independently installed policy without creating or repairing storage.
func loadReservedRuntimePolicy(ctx context.Context, policy supervisor.Policy, images, volumes, channels string, accessGID, transferGID, maintenanceUID, maintenanceGID int) (supervisor.GuestUIDPool, error) {
	empty := supervisor.GuestUIDPool{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if policy.GuestIdentity == nil || policy.Validate() != nil || accessGID < 1 || accessGID > 1<<31-1 || transferGID < 1 || transferGID > 1<<31-1 {
		return empty, supervisor.ErrPolicy
	}
	uids := []uint32{policy.ControllerUID, policy.TransferUID}
	gids := []uint32{uint32(accessGID), uint32(transferGID)}
	if maintenanceUID != -1 || maintenanceGID != -1 {
		if maintenanceUID < 1 || maintenanceUID > 1<<31-1 || maintenanceGID < 1 || maintenanceGID > 1<<31-1 {
			return empty, supervisor.ErrPolicy
		}
		uids = append(uids, uint32(maintenanceUID))
		gids = append(gids, uint32(maintenanceGID))
	}
	identity := *policy.GuestIdentity
	if err := identity.Validate(uids...); err != nil {
		return empty, err
	}
	checkStorage := func() error {
		return supervisor.ObserveReservedStorageParents(ctx, images, volumes, channels, identity.GuestGID, uint32(transferGID))
	}
	if err := checkStorage(); err != nil {
		return empty, err
	}
	pool, err := supervisor.QualifyReservedGuestPolicy(ctx, identity, uids, gids)
	if err != nil {
		return empty, err
	}
	if err := checkStorage(); err != nil {
		return empty, err
	}
	return pool, nil
}
