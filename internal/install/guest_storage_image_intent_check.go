package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
)

// Keeps the original provenance record open throughout recovery. Consumers
// separately qualify catalog, live image descriptors and migration exclusion.
func (e *Engine) withGuestStorageImagesIntentGuarded(ctx context.Context, use func(context.Context, guestStorageImagesIntent, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	const name = "guest-storage-images-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, 8192) {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return err
	}
	var intent guestStorageImagesIntent
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil || intent.Version != 1 {
		return ErrConflict
	}
	canonical, err := canonicalGuestStorageImagesIntent(ctx, intent.Plan, intent.Images)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) || !e.accountJournalPathUnchanged(name, info, 8192) {
		return ErrConflict
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
		return err
	}
	if err := use(ctx, intent, check); err != nil {
		return err
	}
	return check()
}
