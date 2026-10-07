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

// PrepareGuestUIDProvisioning records qualified intent only. It does not alter
// host allocation policy, publish runtime policy, or grant activation authority.
func (e *Engine) PrepareGuestUIDProvisioning(ctx context.Context, pool supervisor.GuestUIDPool) (GuestUIDProvisioningPlan, error) {
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
	plan, err := e.planInstalledGuestUIDProvisioningLocked(ctx, pool)
	if err != nil {
		return empty, err
	}
	if err := e.commitGuestUIDIntent(ctx, plan); err != nil {
		return empty, err
	}
	return plan, nil
}

func (e *Engine) commitGuestUIDIntent(ctx context.Context, plan GuestUIDProvisioningPlan) (result error) {
	qualified, err := guestUIDProvisioningPlan(ctx, plan.OwnerID, supervisor.GuestUIDPool{First: plan.First, Last: plan.Last}, plan.ServiceUIDs)
	if err != nil {
		return err
	}
	actual, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	expected, err := json.Marshal(qualified)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return ErrPlan
	}
	data, err := json.Marshal(struct {
		Version int                      `json:"version"`
		Plan    GuestUIDProvisioningPlan `json:"plan"`
	}{1, qualified})
	if err != nil {
		return err
	}
	if len(data) > 8192 {
		return ErrPlan
	}
	const name = "guest-uid-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	existing := errors.Is(err, os.ErrExist)
	if existing {
		file, err = e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, 8192) {
		return ErrConflict
	}
	if existing {
		contents, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil {
			return err
		}
		if !bytes.Equal(contents, data) {
			return ErrConflict
		}
	} else {
		if e.checkpoint != nil {
			if err := e.checkpoint("guest-uid-intent-created", name); err != nil {
				return err
			}
		}
		// Retain partial writes for explicit recovery; never replace uncertain intent.
		if n, err := file.Write(data); err != nil || n != len(data) {
			return errors.Join(io.ErrShortWrite, err)
		}
		if e.checkpoint != nil {
			if err := e.checkpoint("guest-uid-intent-written", name); err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	final, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(final, e.owner, 8192) || final.Size() != int64(len(data)) || !e.accountJournalPathUnchanged(name, final, 8192) {
		return ErrConflict
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := syncDirectory(e.journalRoot, "."); err != nil {
		return err
	}
	if e.checkpoint != nil {
		if err := e.checkpoint("guest-uid-intent-durable", name); err != nil {
			return err
		}
	}
	return ctx.Err()
}
