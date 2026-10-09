//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"time"
)

// QualifyReservedGuestPolicy observes independent host configuration under the
// shared account writer protocol. Existing guest processes are handled through
// owned runtime reconciliation; this function cannot authorize storage changes.
func QualifyReservedGuestPolicy(ctx context.Context, policy ReservedGuestPolicy, serviceUIDs, serviceGIDs []uint32) (pool GuestUIDPool, result error) {
	if err := ctx.Err(); err != nil {
		return GuestUIDPool{}, err
	}
	if len(serviceUIDs) < 2 || len(serviceGIDs) < 2 || policy.Validate(serviceUIDs...) != nil || os.Geteuid() != 0 {
		return GuestUIDPool{}, ErrPolicy
	}
	for _, gid := range serviceGIDs {
		if gid == 0 || gid > 1<<31-1 || gid == policy.GuestGID {
			return GuestUIDPool{}, ErrPolicy
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer func() {
		if result != nil {
			pool = GuestUIDPool{}
		}
	}()
	host, err := os.OpenRoot("/")
	if err != nil {
		return GuestUIDPool{}, err
	}
	defer func() { result = errors.Join(result, host.Close()) }()
	result = withReservedAccountDirectory(ctx, host, func(ctx context.Context, guard func(context.Context) error) error {
		if err := guard(ctx); err != nil {
			return err
		}
		group, err := ObserveGuestKVMGroup(ctx)
		if err != nil {
			return err
		}
		if group != policy.GuestGID {
			return ErrPolicy
		}
		if err := ObserveGuestUIDNameServiceEligibility(ctx); err != nil {
			return err
		}
		proposed := GuestUIDPool{First: policy.FirstUID, Last: policy.LastUID}
		if err := ObserveGuestUIDAutomaticAllocation(ctx, proposed); err != nil {
			return err
		}
		observed, err := ObserveLocalGuestUIDConflicts(ctx, proposed)
		if err != nil {
			return err
		}
		for _, blocked := range observed.Blocked {
			if blocked {
				return ErrPolicy
			}
		}
		if err := guard(ctx); err != nil {
			return err
		}
		currentGroup, err := ObserveGuestKVMGroup(ctx)
		if err != nil {
			return err
		}
		if currentGroup != group {
			return ErrPolicy
		}
		if err := guard(ctx); err != nil {
			return err
		}
		pool = observed
		return nil
	})
	return pool, result
}
