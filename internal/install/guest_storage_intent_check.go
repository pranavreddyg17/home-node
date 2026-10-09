package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// CheckGuestStorageProvisioningIntent checks saved storage provisioning intent against
// live host eligibility. It neither repairs intent nor certifies exclusive
// allocation, storage migration or runtime policy publication, or guest activation.
func (e *Engine) CheckGuestStorageProvisioningIntent(ctx context.Context) (GuestStorageProvisioningPlan, error) {
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
	return e.withGuestStorageIntent(ctx, func(ctx context.Context, intent GuestStorageProvisioningPlan) error {
		live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: intent.Identity.First, Last: intent.Identity.Last})
		if err != nil {
			return err
		}
		expected, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		observed, err := json.Marshal(live)
		if err != nil {
			return err
		}
		if !bytes.Equal(expected, observed) {
			return ErrConflict
		}
		return nil
	})
}

func (e *Engine) loadGuestStorageIntent(ctx context.Context) (GuestStorageProvisioningPlan, error) {
	return e.withGuestStorageIntent(ctx, nil)
}

// Keep the original record open through potentially slow live host observation.
func (e *Engine) withGuestStorageIntent(ctx context.Context, revalidate func(context.Context, GuestStorageProvisioningPlan) error) (plan GuestStorageProvisioningPlan, result error) {
	return e.withGuestStorageIntentGuarded(ctx, func(ctx context.Context, plan GuestStorageProvisioningPlan, check func() error) error {
		if revalidate != nil {
			return revalidate(ctx, plan)
		}
		return nil
	})
}

// The check closure authenticates the original retained record, never a
// replacement with matching bytes. Consumers must separately retain runtime,
// account-allocation and storage authority before mutation.
func (e *Engine) withGuestStorageIntentGuarded(ctx context.Context, use func(context.Context, GuestStorageProvisioningPlan, func() error) error) (plan GuestStorageProvisioningPlan, result error) {
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	const name = "guest-storage-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return plan, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			plan = GuestStorageProvisioningPlan{}
		}
	}()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, 8192) {
		return plan, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return plan, err
	}
	var intent guestStorageIntent
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil || intent.Version != 1 {
		return plan, ErrConflict
	}
	qualified, err := canonicalGuestStoragePlan(ctx, intent.Plan)
	if err != nil {
		return plan, err
	}
	canonical, err := json.Marshal(guestStorageIntent{Version: 1, Plan: qualified})
	if err != nil {
		return plan, err
	}
	if !bytes.Equal(canonical, data) || !e.accountJournalPathUnchanged(name, info, 8192) {
		return plan, ErrConflict
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		currentBytes, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil {
			return err
		}
		current, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(current, e.owner, 8192) || !os.SameFile(info, current) || current.Size() != info.Size() || current.Mode() != info.Mode() || !bytes.Equal(currentBytes, data) || !e.accountJournalPathUnchanged(name, info, 8192) {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return plan, err
	}
	if use != nil {
		consumerPlan := qualified
		consumerPlan.Identity.ServiceUIDs = append([]uint32(nil), qualified.Identity.ServiceUIDs...)
		consumerPlan.Identity.Pending = append([]string(nil), qualified.Identity.Pending...)
		if err := use(ctx, consumerPlan, check); err != nil {
			return plan, err
		}
	}
	if err := check(); err != nil {
		return plan, err
	}
	return qualified, nil
}
