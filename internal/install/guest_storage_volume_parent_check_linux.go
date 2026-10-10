//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"syscall"
)

// Caller independently authenticates the plan, source group and current installation.
// Parent pathname, mount and ownership authority require a separate retained scope.
// Admit only the exact source or planned destination journal, retaining the
// original transition inode throughout its consumer.
func (e *Engine) withGuestStorageVolumeParentIntent(ctx context.Context, current journal, plan GuestStorageProvisioningPlan, sourceGID uint32, use func(guestStorageVolumeParentIntent, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return err
	}
	const name = "guest-storage-volume-parent-intent.json"
	const maximum = 131072
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, maximum) {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return err
	}
	var intent guestStorageVolumeParentIntent
	if len(data) > maximum || json.Unmarshal(data, &intent) != nil || intent.Version != 1 || !reflect.DeepEqual(intent.Plan, plan) || intent.SourceGID != sourceGID || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 || intent.Original.ID != plan.Identity.OwnerID || len(intent.Original.Items) == 0 || len(intent.Original.Items) > maxInstallationItems {
		return ErrConflict
	}
	desired, err := planGuestStorageVolumeParentJournal(ctx, intent.Original, sourceGID, plan.GuestGID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(desired, intent.Desired) || (!reflect.DeepEqual(current, intent.Original) && !reflect.DeepEqual(current, intent.Desired)) {
		return ErrConflict
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, data) || !e.accountJournalPathUnchanged(name, info, maximum) {
		return ErrConflict
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
		if err != nil {
			return err
		}
		observed, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(observed, e.owner, maximum) || !os.SameFile(info, observed) || observed.Mode() != info.Mode() || observed.Size() != info.Size() || !bytes.Equal(contents, data) || !e.accountJournalPathUnchanged(name, info, maximum) {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	if err := use(intent, check); err != nil {
		return err
	}
	return check()
}
