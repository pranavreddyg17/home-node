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
func (e *Engine) loadGuestIdentityNameServiceIntent(ctx context.Context, ownerID string) (intent guestIdentityNameServiceIntent, result error) {
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	owner, err := hex.DecodeString(ownerID)
	if err != nil || len(owner) != 16 || hex.EncodeToString(owner) != ownerID {
		return intent, ErrPlan
	}
	const name = "guest-identity-nss-intent.json"
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return intent, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			intent = guestIdentityNameServiceIntent{}
		}
	}()
	before, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, 8192) || !e.accountJournalPathUnchanged(name, before, 8192) {
		return intent, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return intent, err
	}
	if len(data) > 8192 || json.Unmarshal(data, &intent) != nil {
		return intent, ErrConflict
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return intent, err
	}
	if !bytes.Equal(data, canonical) || intent.OwnerID != ownerID {
		return intent, ErrConflict
	}
	// Keep the first descriptor open while the retained scope validates the
	// canonical record, recomputed proposal, durability and named identity.
	result = e.withGuestIdentityNameServiceIntent(ctx, intent, func(ctx context.Context) error {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		currentBytes, err := io.ReadAll(io.LimitReader(file, 8193))
		if err != nil {
			return err
		}
		after, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(after, e.owner, 8192) || !os.SameFile(before, after) || after.Size() != int64(len(data)) || !bytes.Equal(currentBytes, data) || !e.accountJournalPathUnchanged(name, before, 8192) {
			return ErrConflict
		}
		return ctx.Err()
	})
	return intent, result
}
