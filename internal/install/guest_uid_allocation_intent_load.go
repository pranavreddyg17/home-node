package install

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
)

// The owner must come from independently inspected installed accounts. Loading
// an intent does not authorize host changes or adopt a staging inode.
func (e *Engine) loadGuestUIDAllocationIntent(ctx context.Context, ownerID string) (intent guestUIDAllocationIntent, result error) {
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	owner, err := hex.DecodeString(ownerID)
	if err != nil || len(owner) != 16 || hex.EncodeToString(owner) != ownerID {
		return intent, ErrPlan
	}
	const name = "guest-uid-allocation-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return intent, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			intent = guestUIDAllocationIntent{}
		}
	}()
	before, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, 262144) || !e.accountJournalPathUnchanged(name, before, 262144) {
		return intent, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 262145))
	if err != nil {
		return intent, err
	}
	if len(data) > 262144 || json.Unmarshal(data, &intent) != nil {
		return intent, ErrConflict
	}
	canonical, err := encodeGuestUIDAllocationIntent(ctx, intent)
	if err != nil {
		return intent, err
	}
	if !bytes.Equal(data, canonical) || intent.OwnerID != ownerID {
		return intent, ErrConflict
	}
	// Keep the first descriptor open while the retained scope validates the
	// canonical record, recomputed proposal, durability and named identity.
	result = e.withGuestUIDAllocationIntent(ctx, intent, func(ctx context.Context, _ func() error) error {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		currentBytes, err := io.ReadAll(io.LimitReader(file, 262145))
		if err != nil {
			return err
		}
		after, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(after, e.owner, 262144) || !os.SameFile(before, after) || after.Size() != int64(len(data)) || !bytes.Equal(currentBytes, data) || !e.accountJournalPathUnchanged(name, before, 262144) {
			return ErrConflict
		}
		return ctx.Err()
	})
	return intent, result
}
