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
	intent, err := e.loadGuestStorageIntent(ctx)
	if err != nil {
		return empty, err
	}
	live, err := e.planInstalledGuestStorageProvisioningLocked(ctx, supervisor.GuestUIDPool{First: intent.Identity.First, Last: intent.Identity.Last})
	if err != nil {
		return empty, err
	}
	expected, _ := json.Marshal(intent)
	observed, _ := json.Marshal(live)
	if !bytes.Equal(expected, observed) {
		return empty, ErrConflict
	}
	// Recheck canonical intent after potentially slow host lookups.
	current, err := e.loadGuestStorageIntent(ctx)
	if err != nil {
		return empty, err
	}
	currentBytes, _ := json.Marshal(current)
	if !bytes.Equal(expected, currentBytes) {
		return empty, ErrConflict
	}
	return live, nil
}

func (e *Engine) loadGuestStorageIntent(ctx context.Context) (plan GuestStorageProvisioningPlan, result error) {
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
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	return qualified, nil
}
