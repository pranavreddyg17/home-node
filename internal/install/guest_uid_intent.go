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
	data, err := json.Marshal(guestUIDIntent{Version: 1, Plan: qualified})
	if err != nil {
		return err
	}
	return e.commitImmutableGuestIntent(ctx, "guest-uid-intent.json", "guest-uid-intent", data)
}

func (e *Engine) commitImmutableGuestIntent(ctx context.Context, name, prefix string, data []byte) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	maximum := int64(8192)
	if name == "guest-uid-allocation-intent.json" || name == "guest-storage-configuration-intent.json" {
		maximum = 262144
	} else if name == "guest-storage-image-parent-journal.json" || name == "guest-storage-volume-parent-intent.json" {
		maximum = 131072
	} else if name != "guest-uid-intent.json" && name != "guest-storage-intent.json" && name != "guest-storage-images-intent.json" && name != "guest-storage-image-parent-intent.json" && name != "guest-identity-nss-intent.json" && name != "guest-identity-nss-stage.json" && name != "guest-uid-allocation-stage.json" && name != "guest-storage-configuration-stage.json" && name != "guest-storage-channel-stage.json" {
		return ErrPlan
	}
	if int64(len(data)) > maximum {
		return ErrPlan
	}
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
	if err != nil || !accountJournalFileAdmitted(info, e.owner, maximum) {
		return ErrConflict
	}
	if existing {
		contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
		if err != nil {
			return err
		}
		if !bytes.Equal(contents, data) {
			return ErrConflict
		}
	} else {
		if e.checkpoint != nil {
			if err := e.checkpoint(prefix+"-created", name); err != nil {
				return err
			}
		}
		// Retain partial writes for explicit recovery; never replace uncertain intent.
		if n, err := file.Write(data); err != nil || n != len(data) {
			return errors.Join(io.ErrShortWrite, err)
		}
		if e.checkpoint != nil {
			if err := e.checkpoint(prefix+"-written", name); err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	final, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(final, e.owner, maximum) || final.Size() != int64(len(data)) || !e.accountJournalPathUnchanged(name, final, maximum) {
		return ErrConflict
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := syncDirectory(e.journalRoot, "."); err != nil {
		return err
	}
	if e.checkpoint != nil {
		if err := e.checkpoint(prefix+"-durable", name); err != nil {
			return err
		}
	}
	return ctx.Err()
}
