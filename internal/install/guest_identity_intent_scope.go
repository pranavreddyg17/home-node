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

// Caller holds installer exclusion and independently qualifies host ownership
// and runtime exclusion. Keep exact intent open and durable through mutation.
func (e *Engine) withGuestIdentityNameServiceIntent(ctx context.Context, intent guestIdentityNameServiceIntent, use func(context.Context) error) (result error) {
	if use == nil {
		return ErrPlan
	}
	return e.withGuestIdentityNameServiceIntentGuarded(ctx, intent, func(ctx context.Context, _ func() error) error { return use(ctx) })
}

func (e *Engine) withGuestIdentityNameServiceIntentGuarded(ctx context.Context, intent guestIdentityNameServiceIntent, use func(context.Context, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, err := hex.DecodeString(intent.OwnerID)
	if use == nil || intent.Version != 1 || err != nil || len(owner) != 16 || hex.EncodeToString(owner) != intent.OwnerID {
		return ErrPlan
	}
	proposal, err := planGuestIdentityNameServices([]byte(intent.Original))
	if err != nil {
		return err
	}
	if proposal != intent.Proposal {
		return ErrConflict
	}
	expected, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return e.withGuestIdentityRecordGuarded(ctx, "guest-identity-nss-intent.json", expected, use)
}

func (e *Engine) withGuestIdentityRecord(ctx context.Context, name string, expected []byte, use func(context.Context) error) (result error) {
	if use == nil {
		return ErrPlan
	}
	return e.withGuestIdentityRecordGuarded(ctx, name, expected, func(ctx context.Context, _ func() error) error { return use(ctx) })
}

func (e *Engine) withGuestIdentityRecordGuarded(ctx context.Context, name string, expected []byte, use func(context.Context, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	maximum := int64(8192)
	if name == "guest-uid-allocation-intent.json" {
		maximum = 262144
	} else if name != "guest-identity-nss-intent.json" && name != "guest-identity-nss-stage.json" && name != "guest-uid-allocation-stage.json" {
		return ErrPlan
	}
	if use == nil || len(expected) == 0 || int64(len(expected)) > maximum {
		return ErrPlan
	}
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	before, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, maximum) || before.Size() != int64(len(expected)) || !e.accountJournalPathUnchanged(name, before, maximum) {
		return ErrConflict
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(file, maximum+1))
		if err != nil {
			return err
		}
		current, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(current, e.owner, maximum) || !os.SameFile(before, current) || current.Size() != before.Size() || !bytes.Equal(data, expected) || !e.accountJournalPathUnchanged(name, before, maximum) {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := syncDirectory(e.journalRoot, "."); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if err := use(ctx, check); err != nil {
		return err
	}
	return check()
}
