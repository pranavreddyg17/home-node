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

// Caller retains independently authenticated image provenance and migration
// exclusion. A matching replacement journal never inherits the original inode's
// authority. The consumer must independently pin and qualify the live parent.
func (e *Engine) withGuestStorageImageParentIntent(ctx context.Context, images guestStorageImagesIntent, sourceGID uint32, use func(guestStorageImageParentIntent, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if images.Version != 1 || sourceGID == 0 || sourceGID > math.MaxInt32 || use == nil {
		return ErrPlan
	}
	imageBytes, err := canonicalGuestStorageImagesIntent(ctx, images.Plan, images.Images)
	if err != nil {
		return err
	}
	for _, image := range images.Images {
		if image.SourceGID != sourceGID {
			return ErrConflict
		}
	}
	const name = "guest-storage-image-parent-intent.json"
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
	var intent guestStorageImageParentIntent
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil || intent.Version != 1 || !reflect.DeepEqual(intent.Plan, images.Plan) || intent.SourceGID != sourceGID || intent.ImagesIntentSHA256 != digest(imageBytes) || intent.Device > math.MaxInt64 || intent.Inode == 0 || intent.Inode > math.MaxInt64 {
		return ErrConflict
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, data) || !e.accountJournalPathUnchanged(name, info, 8192) {
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
	if err := use(intent, check); err != nil {
		return err
	}
	return check()
}
