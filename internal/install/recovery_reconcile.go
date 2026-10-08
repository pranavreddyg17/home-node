package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/backup"
)

// reconcileRecoveryStaging requires an independently qualified expected intent
// and installer exclusion. Existing receipt alone never proves current bytes.
// It does not create missing records, publish files or assign runtime authority.
func (e *Engine) reconcileRecoveryStaging(ctx context.Context, expected recoveryIntent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	installed, err := e.load()
	if err != nil || installed.Phase != "installed" || expected.Version != 1 || installed.ID != expected.ConfigurationID || installed.Digest != expected.ConfigurationDigest {
		return ErrConflict
	}
	if err = e.requireRecoveryActivationBlock(ctx); err != nil {
		return err
	}
	// The journal identifies expected configuration; current owned files must
	// still match before handing recovered bytes to any publication step.
	for _, record := range installed.Items {
		if err = e.matches(record); err != nil {
			return ErrConflict
		}
	}
	intent, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if err = e.matchRecoveryRecord(ctx, "recovery.json", intent); err != nil {
		return err
	}
	receipt, err := json.Marshal(recoveryStagedReceipt{Version: 1, IntentSHA256: digest(intent), Files: len(expected.Recovery.Disks) + 1})
	if err != nil {
		return err
	}
	if err = e.matchRecoveryRecord(ctx, "recovery-staged.json", receipt); err != nil {
		return err
	}
	management := backup.PreparedRecoveryFile{Bytes: expected.Recovery.ManagementBytes, SHA256: expected.Recovery.ManagementSHA256}
	if err = verifyRecoveryCopy(ctx, e.journalRoot, ".recovery-management.copy", management, e.owner); err != nil {
		return err
	}
	for _, disk := range expected.Recovery.Disks {
		if err = verifyRecoveryCopy(ctx, e.journalRoot, ".recovery-"+disk.InstanceID+".copy", backup.PreparedRecoveryFile{Bytes: disk.Bytes, SHA256: disk.SourceSHA256}, e.owner); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (e *Engine) matchRecoveryRecord(ctx context.Context, name string, expected []byte) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if (name != "recovery.json" && name != "recovery-staged.json") || len(expected) == 0 || len(expected) > 8192 {
		return ErrPlan
	}
	before, err := e.journalRoot.Lstat(name)
	if err != nil {
		return err
	}
	if !accountJournalFileAdmitted(before, e.owner, 8192) || before.Size() != int64(len(expected)) {
		return ErrConflict
	}
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !accountJournalFileAdmitted(opened, e.owner, 8192) || opened.Size() != int64(len(expected)) {
		return ErrConflict
	}
	actual, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || !bytes.Equal(actual, expected) {
		return ErrConflict
	}
	current, err := e.journalRoot.Lstat(name)
	if err != nil || !os.SameFile(opened, current) || current.Size() != int64(len(expected)) || !accountJournalFileAdmitted(current, e.owner, 8192) {
		return ErrConflict
	}
	return ctx.Err()
}
