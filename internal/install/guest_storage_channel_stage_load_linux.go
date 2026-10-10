//go:build linux

package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// A loaded record describes provenance, not publication authority. Its consumer
// must retain the record again and qualify the actual runtime/candidate inodes.
func (e *Engine) loadGuestStorageChannelStage(ctx context.Context, plan GuestStorageProvisioningPlan, transferGID uint32) (stage guestStorageChannelStage, result error) {
	if err := ctx.Err(); err != nil {
		return stage, err
	}
	if e == nil || e.journalRoot == nil {
		return stage, ErrPlan
	}
	const name = "guest-storage-channel-stage.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return stage, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			stage = guestStorageChannelStage{}
		}
	}()
	before, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, 8192) || !e.accountJournalPathUnchanged(name, before, 8192) {
		return stage, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return stage, err
	}
	if len(data) > 8192 || json.Unmarshal(data, &stage) != nil {
		return stage, ErrConflict
	}
	canonical, err := json.Marshal(stage)
	if err != nil {
		return stage, err
	}
	if !bytes.Equal(data, canonical) {
		return stage, ErrConflict
	}
	if err := validateGuestStorageChannelStage(ctx, plan, transferGID, stage); err != nil {
		return stage, errors.Join(ErrConflict, err)
	}
	result = e.withGuestIdentityRecordGuarded(ctx, name, canonical, func(ctx context.Context, checkRecord func() error) error {
		if err := checkRecord(); err != nil {
			return err
		}
		if !e.accountJournalPathUnchanged(name, before, 8192) {
			return ErrConflict
		}
		current, err := io.ReadAll(io.NewSectionReader(file, 0, 8193))
		if err != nil {
			return err
		}
		if !bytes.Equal(current, canonical) {
			return ErrConflict
		}
		return checkRecord()
	})
	return stage, result
}
