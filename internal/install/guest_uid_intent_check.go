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

type guestUIDIntent struct {
	Version int                      `json:"version"`
	Plan    GuestUIDProvisioningPlan `json:"plan"`
}

// CheckGuestUIDProvisioningIntent checks saved first-provisioning intent against
// live host eligibility. It neither repairs intent nor certifies exclusive
// allocation, runtime policy publication, or guest activation.
func (e *Engine) CheckGuestUIDProvisioningIntent(ctx context.Context) (GuestUIDProvisioningPlan, error) {
	empty := GuestUIDProvisioningPlan{}
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
	intent, err := e.loadGuestUIDIntent(ctx)
	if err != nil {
		return empty, err
	}
	live, err := e.planInstalledGuestUIDProvisioningLocked(ctx, supervisor.GuestUIDPool{First: intent.First, Last: intent.Last})
	if err != nil {
		return empty, err
	}
	expected, _ := json.Marshal(intent)
	observed, _ := json.Marshal(live)
	if !bytes.Equal(expected, observed) {
		return empty, ErrConflict
	}
	// Recheck canonical intent after potentially slow host lookups.
	current, err := e.loadGuestUIDIntent(ctx)
	if err != nil {
		return empty, err
	}
	currentBytes, _ := json.Marshal(current)
	if !bytes.Equal(expected, currentBytes) {
		return empty, ErrConflict
	}
	return live, nil
}

func (e *Engine) loadGuestUIDIntent(ctx context.Context) (plan GuestUIDProvisioningPlan, result error) {
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	const name = "guest-uid-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return plan, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			plan = GuestUIDProvisioningPlan{}
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
	var intent guestUIDIntent
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil || intent.Version != 1 {
		return plan, ErrConflict
	}
	qualified, err := guestUIDProvisioningPlan(ctx, intent.Plan.OwnerID, supervisor.GuestUIDPool{First: intent.Plan.First, Last: intent.Plan.Last}, intent.Plan.ServiceUIDs)
	if err != nil {
		return plan, err
	}
	canonical, err := json.Marshal(guestUIDIntent{Version: 1, Plan: qualified})
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
