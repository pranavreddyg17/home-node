package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"syscall"
)

// Caller independently authenticates current installation and parent authority.
// Admit only the exact source or planned destination journal, retaining the
// original transition inode throughout its consumer.
func (e *Engine) withGuestStorageParentJournalIntent(ctx context.Context, current journal, sourceGID, guestGID uint32, parentSHA string, use func(guestStorageParentJournalIntent, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil || len(parentSHA) != 64 || strings.Trim(parentSHA, "0123456789abcdef") != "" {
		return ErrPlan
	}
	const name = "guest-storage-image-parent-journal.json"
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
	var intent guestStorageParentJournalIntent
	if len(data) > maximum || json.Unmarshal(data, &intent) != nil || intent.Version != 1 || intent.ParentIntentSHA256 != parentSHA {
		return ErrConflict
	}
	desired, err := planGuestStorageImageParentJournal(ctx, intent.Original, sourceGID, guestGID)
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
